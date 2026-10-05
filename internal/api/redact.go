package api

import "regexp"

var sensitive = regexp.MustCompile(`(?i)token|secret|password|passwd|pwd|private.?key|authorization|csrf|access.?key|credential|signature`)
var urlSecret = regexp.MustCompile(`(?i)([?&](?:[^=&\s]*(?:token|secret|password|passwd|signature|credential|key)[^=&\s]*)=)[^&#\s]+`)
var userInfo = regexp.MustCompile(`(https?://)[^/\s:]+:[^/@\s]+@`)
var pem = regexp.MustCompile(`(?s)-----BEGIN [^-]*PRIVATE KEY-----.*?-----END [^-]*PRIVATE KEY-----`)

func Redact(value any) any {
	switch v := value.(type) {
	case map[string]any:
		env := false
		for _, k := range []string{"name", "key"} {
			if name, ok := v[k].(string); ok && sensitive.MatchString(name) {
				env = true
			}
		}
		out := make(map[string]any, len(v))
		for k, val := range v {
			_, boolean := val.(bool)
			if (sensitive.MatchString(k) && val != nil && !boolean) || (env && k == "value") {
				out[k] = "[REDACTED]"
			} else {
				out[k] = Redact(val)
			}
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i := range v {
			out[i] = Redact(v[i])
		}
		return out
	case string:
		v = userInfo.ReplaceAllString(v, "${1}[REDACTED]@")
		v = urlSecret.ReplaceAllString(v, "${1}[REDACTED]")
		return pem.ReplaceAllString(v, "[REDACTED]")
	default:
		return value
	}
}
