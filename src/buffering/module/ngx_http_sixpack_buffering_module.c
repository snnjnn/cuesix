#include <ngx_config.h>
#include <ngx_core.h>
#include <ngx_http.h>

/*
 * Native return codes consumed by the Lua FFI wrapper.
 * We keep this integer-based contract stable because FFI calls cannot return
 * Nginx status objects directly.
 */
#define NGX_HTTP_SIXPACK_BUFFERING_RC_OK 0
#define NGX_HTTP_SIXPACK_BUFFERING_RC_DEFERRED 1
#define NGX_HTTP_SIXPACK_BUFFERING_RC_BAD_ARG -1
#define NGX_HTTP_SIXPACK_BUFFERING_RC_NO_REQUEST -2
#define NGX_HTTP_SIXPACK_BUFFERING_RC_NO_MEMORY -3

/*
 * Per-request module context.
 *
 * This is stored on ngx_http_request_t via:
 * - ngx_http_get_module_ctx(r, module)
 * - ngx_http_set_ctx(r, ctx, module)
 *
 * Nginx keeps this pointer for the request lifetime.
 */
typedef struct {
    unsigned has_intent : 1;
    unsigned buffering : 1;
} ngx_http_sixpack_buffering_ctx_t;

/* Module symbol used by ctx getters/setters. */
extern ngx_module_t ngx_http_sixpack_buffering_module;

/*
 * Header filter chain integration.
 *
 * Nginx HTTP filters are a linked chain where each filter calls the next one.
 * We store the previous top filter and register ours as the new top.
 */
static ngx_http_output_header_filter_pt ngx_http_sixpack_buffering_next_header_filter;

static ngx_int_t ngx_http_sixpack_buffering_init(ngx_conf_t *cf);
static ngx_int_t ngx_http_sixpack_buffering_header_filter(ngx_http_request_t *r);

/*
 * FFI entrypoint called from Lua (resty.sixpack).
 *
 * Input:
 * - r: current ngx_http_request_t (from resty.core.base.get_request())
 * - enabled: 0/1
 *
 * Behavior:
 * 1) If upstream already exists, apply immediately to r->upstream->buffering.
 * 2) Otherwise persist caller intent in request ctx and return DEFERRED.
 * 3) Header filter will apply deferred intent later.
 */
__attribute__((visibility("default")))
int
ngx_http_sixpack_set_proxy_buffering(ngx_http_request_t *r, int enabled)
{
    ngx_http_sixpack_buffering_ctx_t *ctx;

    if (r == NULL) {
        return NGX_HTTP_SIXPACK_BUFFERING_RC_NO_REQUEST;
    }

    if (!(enabled == 0 || enabled == 1)) {
        return NGX_HTTP_SIXPACK_BUFFERING_RC_BAD_ARG;
    }

    /*
     * Fast path: upstream already exists, so no deferred state is needed.
     */
    if (r->upstream != NULL) {
        r->upstream->buffering = (ngx_uint_t) enabled;
        return NGX_HTTP_SIXPACK_BUFFERING_RC_OK;
    }

    ctx = ngx_http_get_module_ctx(r, ngx_http_sixpack_buffering_module);
    if (ctx == NULL) {
        /*
         * ngx_pcalloc allocates from request pool (r->pool), so memory is
         * automatically reclaimed when the request finishes.
         */
        ctx = ngx_pcalloc(r->pool, sizeof(ngx_http_sixpack_buffering_ctx_t));
        if (ctx == NULL) {
            return NGX_HTTP_SIXPACK_BUFFERING_RC_NO_MEMORY;
        }
        ngx_http_set_ctx(r, ctx, ngx_http_sixpack_buffering_module);
    }

    ctx->has_intent = 1;
    ctx->buffering = (unsigned) enabled;
    return NGX_HTTP_SIXPACK_BUFFERING_RC_DEFERRED;
}

static ngx_int_t
ngx_http_sixpack_buffering_init(ngx_conf_t *cf)
{
    (void) cf;

    /*
     * postconfiguration hook:
     * install our header filter once config parsing is complete.
     */
    ngx_http_sixpack_buffering_next_header_filter = ngx_http_top_header_filter;
    ngx_http_top_header_filter = ngx_http_sixpack_buffering_header_filter;

    return NGX_OK;
}

/*
 * Header filter phase.
 *
 * At this point, proxied requests typically have r->upstream initialized.
 * If Lua called set_proxy_buffering before upstream existed, we apply the
 * deferred intent here.
 */
static ngx_int_t
ngx_http_sixpack_buffering_header_filter(ngx_http_request_t *r)
{
    ngx_http_sixpack_buffering_ctx_t *ctx;

    ctx = ngx_http_get_module_ctx(r, ngx_http_sixpack_buffering_module);
    if (ctx != NULL && ctx->has_intent && r->upstream != NULL) {
        r->upstream->buffering = (ngx_uint_t) ctx->buffering;
        ctx->has_intent = 0;
    }

    return ngx_http_sixpack_buffering_next_header_filter(r);
}

/*
 * Minimal HTTP module definition.
 *
 * We do not expose directives; only postconfiguration is needed to hook
 * the header filter chain and expose the exported FFI symbol.
 */
static ngx_http_module_t ngx_http_sixpack_buffering_module_ctx = {
    NULL,                            /* preconfiguration */
    ngx_http_sixpack_buffering_init,  /* postconfiguration */

    NULL, /* create main configuration */
    NULL, /* init main configuration */

    NULL, /* create server configuration */
    NULL, /* merge server configuration */

    NULL, /* create location configuration */
    NULL  /* merge location configuration */
};

ngx_module_t ngx_http_sixpack_buffering_module = {
    NGX_MODULE_V1,
    &ngx_http_sixpack_buffering_module_ctx, /* module context */
    NULL,                                  /* module directives */
    NGX_HTTP_MODULE,                       /* module type */
    NULL,                                  /* init master */
    NULL,                                  /* init module */
    NULL,                                  /* init process */
    NULL,                                  /* init thread */
    NULL,                                  /* exit thread */
    NULL,                                  /* exit process */
    NULL,                                  /* exit master */
    NGX_MODULE_V1_PADDING
};
