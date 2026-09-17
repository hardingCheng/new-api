package tencentmps

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// credentials 是渠道密钥框里的四段：SecretId|SecretKey|Region|Bucket。
// 前两段签请求；后两段是账号下一个真实存在的 COS 桶及其地域——输出走临时链接时
// 上游仍要求填一个能过校验的输出桶。
type credentials struct {
	SecretID  string
	SecretKey string
	Region    string
	Bucket    string
}

func parseChannelKey(key string) (credentials, error) {
	parts := strings.Split(key, "|")
	if len(parts) != 4 {
		return credentials{}, errors.New("channel key must be SecretId|SecretKey|Region|Bucket")
	}
	cred := credentials{
		SecretID:  strings.TrimSpace(parts[0]),
		SecretKey: strings.TrimSpace(parts[1]),
		Region:    strings.TrimSpace(parts[2]),
		Bucket:    strings.TrimSpace(parts[3]),
	}
	if cred.SecretID == "" || cred.SecretKey == "" || cred.Region == "" || cred.Bucket == "" {
		return credentials{}, errors.New("channel key must be SecretId|SecretKey|Region|Bucket, none of them empty")
	}
	return cred, nil
}

// endpointHost 取出签名要用的 host；渠道没填 base url 时用默认公网接入点。
func endpointHost(baseURL string) (string, error) {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultEndpoint
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", errors.Wrap(err, "invalid base url")
	}
	if u.Host == "" {
		return "", fmt.Errorf("invalid base url: %s", baseURL)
	}
	return u.Host, nil
}

// signRequest 给腾讯云 API 3.0 请求加上 TC3-HMAC-SHA256 签名及公共头。
// payload 必须与实际发送的请求体逐字节一致。
func signRequest(req *http.Request, cred credentials, action string, payload []byte, now time.Time) {
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Host", req.URL.Host)
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Version", apiVersion)
	req.Header.Set("X-TC-Timestamp", strconv.FormatInt(now.Unix(), 10))
	req.Header.Set("Authorization", buildAuthorization(cred.SecretID, cred.SecretKey, serviceName, req.URL.Host, action, payload, now))
}

// buildAuthorization 组装 Authorization 头。算法见腾讯云「签名方法 v3」。
func buildAuthorization(secretID, secretKey, service, host, action string, payload []byte, now time.Time) string {
	const algorithm = "TC3-HMAC-SHA256"
	const signedHeaders = "content-type;host;x-tc-action"

	utc := now.UTC()
	date := utc.Format("2006-01-02")
	credentialScope := fmt.Sprintf("%s/%s/tc3_request", date, service)

	stringToSign := fmt.Sprintf("%s\n%d\n%s\n%s",
		algorithm,
		utc.Unix(),
		credentialScope,
		sha256Hex([]byte(canonicalRequest(host, action, payload))))

	secretDate := hmacSHA256([]byte("TC3"+secretKey), date)
	secretService := hmacSHA256(secretDate, service)
	secretSigning := hmacSHA256(secretService, "tc3_request")
	signature := hex.EncodeToString(hmacSHA256(secretSigning, stringToSign))

	return fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		algorithm, secretID, credentialScope, signedHeaders, signature)
}

// canonicalRequest 是签名规范里的 CanonicalRequest：固定 POST / 、三个签名头、请求体哈希。
func canonicalRequest(host, action string, payload []byte) string {
	return canonicalRequestForPayloadHash(host, action, sha256Hex(payload))
}

func canonicalRequestForPayloadHash(host, action, payloadHash string) string {
	return strings.Join([]string{
		"POST",
		"/",
		"",
		"content-type:application/json; charset=utf-8",
		"host:" + host,
		"x-tc-action:" + strings.ToLower(action),
		"",
		"content-type;host;x-tc-action",
		payloadHash,
	}, "\n")
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key []byte, msg string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(msg))
	return mac.Sum(nil)
}
