use strict;
use warnings;

use HTTP::Tiny;
use IO::Socket::INET;
use Test::More;
use Time::HiRes qw(time);

my $apisix_host = $ENV{APISIX_HOST} // 'apisix';
my $apisix_port = $ENV{APISIX_PORT} // 9080;
my $apisix_log_file = $ENV{APISIX_LOG_FILE} // '/tmp/apisix-logs/error.log';
my $large_curl_args = $ENV{LARGE_CURL_ARGS} // '--limit-rate 1m';
my $base = "http://${apisix_host}:${apisix_port}";

my $http = HTTP::Tiny->new(
    timeout  => 60,
    keep_alive => 0,
);

# Count APISIX warning lines that indicate response buffering spilled to temp files.
# We compare before/after counts to verify buffering mode behavior per route.
sub count_spool_logs {
    open my $fh, '<', $apisix_log_file or die "failed to read ${apisix_log_file}: $!";
    my $count = 0;
    while (my $line = <$fh>) {
        $count++ if index($line, 'an upstream response is buffered to a temporary file') >= 0;
    }
    close $fh;
    return $count;
}

# Load the full APISIX error log so tests can assert absence of critical errors.
sub apisix_logs {
    open my $fh, '<', $apisix_log_file or die "failed to read ${apisix_log_file}: $!";
    local $/;
    my $logs = <$fh>;
    close $fh;
    return $logs // '';
}

# Execute a large GET with a throttled client to provoke buffering behavior reliably.
# Returns true when curl exits successfully.
sub get_large {
    my ($path) = @_;
    my $url = "${base}${path}";
    my $cmd = "curl -fsS ${large_curl_args} '$url' -o /dev/null";
    my $rc = system($cmd);
    return $rc == 0;
}

# Send a fixed-size POST body to the upload route and return HTTP::Tiny response.
sub post_upload {
    my $payload = "\0" x (256 * 1024);
    return $http->request('POST', "${base}/nobuf/upload", {
        headers => {
            'content-type' => 'application/octet-stream',
            'content-length' => length($payload),
        },
        content => $payload,
    });
}

# Open a raw socket to SSE route, read first event line, and measure time-to-first-event.
# This checks that disabled buffering has immediate effect for streaming responses.
sub first_sse_event_elapsed_ms {
    my $sock = IO::Socket::INET->new(
        PeerHost => $apisix_host,
        PeerPort => $apisix_port,
        Proto    => 'tcp',
        Timeout  => 6,
    ) or die "failed to connect to APISIX for SSE: $!";

    my $start = time();
    my $req = "GET /nobuf/sse HTTP/1.1\r\nHost: ${apisix_host}\r\nAccept: text/event-stream\r\nConnection: close\r\n\r\n";
    print {$sock} $req or die "failed to write SSE request: $!";

    while (my $line = <$sock>) {
        last if $line eq "\r\n";
    }

    my $first_line;
    while (my $line = <$sock>) {
        if ($line =~ /^data:\s*/) {
            $first_line = $line;
            last;
        }
    }

    my $elapsed_ms = int((time() - $start) * 1000);
    close $sock;

    return ($first_line // '', $elapsed_ms);
}

# Generic runner for routes where expected outcome is a spool-log delta rule.
# `positive` means buffering happened; `zero` means no additional buffering warnings.
sub run_spool_delta_case {
    my ($case) = @_;
    my $before = count_spool_logs();
    my $ok = get_large($case->{path});
    ok($ok, "$case->{name}: request succeeds");

    my $after = count_spool_logs();
    my $delta = $after - $before;

    if ($case->{expect_delta} eq 'positive') {
        cmp_ok($delta, '>', 0, "$case->{name}: spool log delta is positive");
    } elsif ($case->{expect_delta} eq 'zero') {
        is($delta, 0, "$case->{name}: spool log delta is zero");
    } else {
        die "unknown delta rule: $case->{expect_delta}";
    }
}

# Generic runner for SSE timing assertions.
sub run_sse_case {
    my ($case) = @_;
    my ($first_line, $elapsed_ms) = first_sse_event_elapsed_ms();
    like($first_line, qr/^data:\s*/, "$case->{name}: SSE emits a data line first");
    cmp_ok($elapsed_ms, '<=', $case->{max_ms}, "$case->{name}: first event arrives within $case->{max_ms}ms");
}

# Generic runner for upload behavior assertions.
sub run_upload_case {
    my ($case) = @_;
    my $upload = post_upload();
    is($upload->{status}, $case->{expected_status}, "$case->{name}: request succeeds");
    like($upload->{content} // '', $case->{body_like}, "$case->{name}: response body matches");
}

# Generic runner for log guardrails where a pattern must not appear.
sub run_logs_absent_case {
    my ($case) = @_;
    my $logs = apisix_logs();
    unlike($logs, $case->{pattern}, $case->{name});
}

my @cases = (
    # Baseline route uses default proxy buffering, so large response should increase spool warnings.
    {
        type         => 'spool_delta',
        name         => 'baseline large response produces spool warnings',
        path         => '/baseline/large',
        expect_delta => 'positive',
    },
    # No-buffer route calls set_proxy_buffering(false) in header_filter, so no extra spool warning is expected.
    {
        type         => 'spool_delta',
        name         => 'no-buffer large response avoids additional spool warnings',
        path         => '/nobuf/large',
        expect_delta => 'zero',
    },
    # Regression check: baseline route must still behave with default buffering after no-buffer traffic.
    {
        type         => 'spool_delta',
        name         => 'baseline large response keeps default buffering after no-buffer request',
        path         => '/baseline/large',
        expect_delta => 'positive',
    },
    # Streaming check: first SSE data line should arrive quickly when buffering is disabled.
    {
        type   => 'sse',
        name   => 'SSE endpoint',
        max_ms => 2000,
    },
    # Coexistence check: request buffering disabled on upload route still accepts body and returns expected size.
    {
        type            => 'upload',
        name            => 'upload route',
        expected_status => 200,
        body_like       => qr/uploaded 262144/,
    },
    # Guardrail: module call in header_filter must not fail.
    {
        type    => 'logs_absent',
        name    => 'APISIX logs do not report set_proxy_buffering failure',
        pattern => qr/set_proxy_buffering failed:/,
    },
    # Guardrail: header_filter invocation must be immediate mode, not delayed mode.
    {
        type    => 'logs_absent',
        name    => 'APISIX logs do not report non-immediate mode in header_filter',
        pattern => qr/set_proxy_buffering expected immediate mode in header_filter/,
    },
);

for my $case (@cases) {
    if ($case->{type} eq 'spool_delta') {
        run_spool_delta_case($case);
    } elsif ($case->{type} eq 'sse') {
        run_sse_case($case);
    } elsif ($case->{type} eq 'upload') {
        run_upload_case($case);
    } elsif ($case->{type} eq 'logs_absent') {
        run_logs_absent_case($case);
    } else {
        die "unknown case type: $case->{type}";
    }
}

done_testing();
