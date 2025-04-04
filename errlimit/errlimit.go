// DigitalOcean SDK uses regular Golang error type
// Unfortunately, the SDK can return length errors
// For example:
// level=warn ts=2025-04-03T18:57:13.262971571Z caller=key.go:57 msg="can't list keys" err="GET https://api.digitalocean.com/v2/account/keys: 504 <!DOCTYPE html>\n<html>\n<head>\n    <title>DigitalOcean - Something went wrong!</title>\n
// This error included base64-encoded SVGs

package errlimit

func Error(err error) string {
	if err == nil {
		return ""
	}

	msg := err.Error()
	if len(msg) < 50 {
		return msg
	}

	// Truncate error message
	return msg[:50] + "..."

}
