local ffi = require("ffi")
local base = require("resty.core.base")

local C = ffi.C
local get_request = base.get_request

ffi.cdef([[
int ngx_http_sixpack_set_proxy_buffering(void *r, int enabled);
]])

local ERRORS = {
  [-1] = "bad argument: enabled must be boolean",
  [-2] = "request context unavailable",
  [-3] = "native runtime failure: unable to allocate request context",
}

local _M = {}

function _M.set_proxy_buffering(enabled)
  if type(enabled) ~= "boolean" then
    return nil, "bad argument: enabled must be boolean"
  end

  local r = get_request()
  if r == nil then
    return nil, "request context unavailable"
  end

  local ok, rc = pcall(C.ngx_http_sixpack_set_proxy_buffering, r, enabled and 1 or 0)
  if not ok then
    return nil, "native runtime unavailable: ngx_http_sixpack_buffering_module not loaded"
  end

  if rc == 0 then
    return true, "immediate"
  end

  if rc == 1 then
    return true, "deferred"
  end

  return nil, ERRORS[rc] or ("native runtime failure rc=" .. tostring(rc))
end

return _M
