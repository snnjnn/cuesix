use strict;
use warnings;

use Test::Nginx::Socket -Base;

repeat_each(1);
no_shuffle();
plan tests => repeat_each() * blocks() * 2;
run_tests();

__DATA__

=== TEST 1: baseline ping proxies to upstream
--- config
    location /baseline/ {
        proxy_pass http://apisix:9080;
    }
--- request
GET /baseline/ping
--- error_code: 200
--- response_body_like: ^pong\n$

=== TEST 2: no-buffer upload returns uploaded byte count
--- config
    location = /nobuf/upload {
        proxy_pass http://apisix:9080;
    }
--- request
POST /nobuf/upload
abcdefghijklmnopqrstuvwxyz
--- more_headers
Content-Type: application/octet-stream
--- error_code: 200
--- response_body_like: uploaded 26
