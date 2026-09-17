package tencentmps

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// 腾讯云「签名方法 v3」文档给出的公开算例（cvm DescribeInstances，时间戳 1551113065）。
// 文档把密钥打了码，且示例请求体经过文档渲染后字节不可复现，所以拿文档给出的
// 请求体哈希去核对规范请求串的拼法：头的顺序、大小写、空行、分隔符任何一处错都对不上。
const docPayloadHash = "35e9c5b0e3ae67532d3c9f17ead6c90222632e5b1ff7f6e89887f1398934f064"
const docCanonicalHash = "7019a55be8395899b900fb5564e4200d984910f34794a27cb3fb7d10ff6a1e84"

var docPayload = []byte(`{"Limit": 1, "Filters": [{"Values": ["未命名"], "Name": "instance-name"}]}`)

func TestCanonicalRequestMatchesOfficialExample(t *testing.T) {
	canonical := canonicalRequestForPayloadHash("cvm.tencentcloudapi.com", "DescribeInstances", docPayloadHash)
	want := "POST\n/\n\ncontent-type:application/json; charset=utf-8\nhost:cvm.tencentcloudapi.com\nx-tc-action:describeinstances\n\ncontent-type;host;x-tc-action\n" + docPayloadHash
	if canonical != want {
		t.Fatalf("canonical request =\n%s\nwant\n%s", canonical, want)
	}
	if got := sha256Hex([]byte(canonical)); got != docCanonicalHash {
		t.Fatalf("canonical request hash = %s", got)
	}
}

func TestCanonicalRequestHashesPayload(t *testing.T) {
	canonical := canonicalRequest("mps.tencentcloudapi.com", "ProcessMedia", docPayload)
	if !strings.HasSuffix(canonical, "\n"+sha256Hex(docPayload)) {
		t.Fatalf("canonical request must end with the payload hash:\n%s", canonical)
	}
	if !strings.Contains(canonical, "x-tc-action:processmedia\n") {
		t.Fatalf("action must be lower-cased in the canonical headers:\n%s", canonical)
	}
}

func TestBuildAuthorizationShape(t *testing.T) {
	now := time.Unix(1551113065, 0)
	auth := buildAuthorization("AKIDexample", "secret", "cvm", "cvm.tencentcloudapi.com", "DescribeInstances", docPayload, now)
	wantPrefix := "TC3-HMAC-SHA256 Credential=AKIDexample/2019-02-25/cvm/tc3_request, SignedHeaders=content-type;host;x-tc-action, Signature="
	if !strings.HasPrefix(auth, wantPrefix) {
		t.Fatalf("authorization = %s", auth)
	}
	sig := strings.TrimPrefix(auth, wantPrefix)
	if len(sig) != 64 {
		t.Fatalf("signature should be 64 hex chars, got %q", sig)
	}
	// 同输入必须得到同签名；换密钥必须变
	if again := buildAuthorization("AKIDexample", "secret", "cvm", "cvm.tencentcloudapi.com", "DescribeInstances", docPayload, now); again != auth {
		t.Fatalf("signature not deterministic")
	}
	if other := buildAuthorization("AKIDexample", "secret2", "cvm", "cvm.tencentcloudapi.com", "DescribeInstances", docPayload, now); other == auth {
		t.Fatalf("signature must depend on the secret key")
	}
}

func TestSignRequestSetsCommonHeaders(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, defaultEndpoint+"/", nil)
	cred := credentials{SecretID: "id", SecretKey: "key", Region: "ap-guangzhou", Bucket: "b-1250000000"}
	signRequest(req, cred, actionProcessMedia, []byte(`{}`), time.Unix(1700000000, 0))

	if got := req.Header.Get("X-TC-Action"); got != "ProcessMedia" {
		t.Fatalf("X-TC-Action = %q", got)
	}
	if got := req.Header.Get("X-TC-Version"); got != apiVersion {
		t.Fatalf("X-TC-Version = %q", got)
	}
	if got := req.Header.Get("X-TC-Timestamp"); got != "1700000000" {
		t.Fatalf("X-TC-Timestamp = %q", got)
	}
	if got := req.Header.Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if !strings.Contains(req.Header.Get("Authorization"), "Credential=id/2023-11-14/mps/tc3_request") {
		t.Fatalf("Authorization = %q", req.Header.Get("Authorization"))
	}
}

func TestParseChannelKey(t *testing.T) {
	cred, err := parseChannelKey(" AKIDxxx | sk | ap-guangzhou | my-bucket-1250000000 ")
	if err != nil {
		t.Fatal(err)
	}
	if cred.SecretID != "AKIDxxx" || cred.SecretKey != "sk" || cred.Region != "ap-guangzhou" || cred.Bucket != "my-bucket-1250000000" {
		t.Fatalf("unexpected credentials: %+v", cred)
	}
	for _, bad := range []string{"", "onlykey", "id|sk", "id|sk|region", "id|sk||bucket", "id|sk|region|bucket|extra"} {
		if _, err := parseChannelKey(bad); err == nil {
			t.Fatalf("key %q should be rejected", bad)
		}
	}
}

func TestEndpointHost(t *testing.T) {
	host, err := endpointHost("")
	if err != nil || host != "mps.tencentcloudapi.com" {
		t.Fatalf("default host = %q, err = %v", host, err)
	}
	host, err = endpointHost("https://mps.ap-shanghai.tencentcloudapi.com/")
	if err != nil || host != "mps.ap-shanghai.tencentcloudapi.com" {
		t.Fatalf("host = %q, err = %v", host, err)
	}
	if _, err := endpointHost("not a url"); err == nil {
		t.Fatal("garbage base url should fail")
	}
}
