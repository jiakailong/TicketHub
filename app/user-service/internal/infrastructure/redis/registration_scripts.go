package redis

import "tickethub/pkg/cache"

var registrationPrecheckScript = cache.LuaScript{
	Name: "registration_precheck",
	Body: `
local now = redis.call('TIME')
local second = now[1]
local minute = math.floor(tonumber(now[1]) / 60)
local base = KEYS[1]
local ip = ARGV[1]
local mobile = ARGV[2]
local qpsLimit = tonumber(ARGV[3])
local ipMinuteLimit = tonumber(ARGV[4])
local mobileMinuteLimit = tonumber(ARGV[5])

local function increment(key, ttl)
  local count = redis.call('INCR', key)
  if count == 1 then
    redis.call('EXPIRE', key, ttl)
  end
  return count
end

local qps = increment(base .. ':register:qps:' .. ip .. ':' .. second, 2)
local ipMinute = increment(base .. ':register:minute:ip:' .. ip .. ':' .. minute, 120)
local mobileMinute = increment(base .. ':register:minute:mobile:' .. mobile .. ':' .. minute, 120)
if ipMinute > ipMinuteLimit or mobileMinute > mobileMinuteLimit then
  return 2
end
if qps >= qpsLimit then
  return 1
end
return 0
`,
}

var registrationIssueCaptchaScript = cache.LuaScript{
	Name: "registration_issue_captcha",
	Body: `
local now = redis.call('TIME')
local minute = math.floor(tonumber(now[1]) / 60)
local base = KEYS[1]
local ip = ARGV[1]
local captchaID = ARGV[2]
local answer = ARGV[3]
local binding = ARGV[4]
local attempts = tonumber(ARGV[5])
local ttl = tonumber(ARGV[6])
local issueLimit = tonumber(ARGV[7])

local issueKey = base .. ':register:captcha:issue:' .. ip .. ':' .. minute
local count = redis.call('INCR', issueKey)
if count == 1 then
  redis.call('EXPIRE', issueKey, 120)
end
if count > issueLimit then
  return 0
end

local captchaKey = base .. ':register:captcha:' .. captchaID
redis.call('HSET', captchaKey, 'answer_hmac', answer, 'binding', binding, 'attempts_left', attempts)
redis.call('EXPIRE', captchaKey, ttl)
return 1
`,
}

var registrationVerifyCaptchaScript = cache.LuaScript{
	Name: "registration_verify_captcha",
	Body: `
if redis.call('EXISTS', KEYS[1]) == 0 then
  return 0
end
if redis.call('HGET', KEYS[1], 'binding') ~= ARGV[1] or redis.call('HGET', KEYS[1], 'answer_hmac') ~= ARGV[2] then
  local attempts = redis.call('HINCRBY', KEYS[1], 'attempts_left', -1)
  if attempts <= 0 then
    redis.call('DEL', KEYS[1])
  end
  return 0
end
redis.call('DEL', KEYS[1])
return 1
`,
}

var registrationReleaseBootstrapLockScript = cache.LuaScript{
	Name: "registration_release_bootstrap_lock",
	Body: `
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('DEL', KEYS[1])
end
return 0
`,
}
