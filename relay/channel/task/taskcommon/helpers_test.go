package taskcommon

import "testing"

func TestExtractReferenceVideoURLsFromBody(t *testing.T) {
	body := []byte(`{
		"content": [
			{"type":"text","text":"hello"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://example.com/a.png"}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"https://example.com/ref.mp4?token=1"}},
			{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://example.com/ref.mp3"}}
		]
	}`)

	urls := ExtractReferenceVideoURLsFromBody(body)
	if len(urls) != 1 {
		t.Fatalf("len(urls) = %d, want 1: %#v", len(urls), urls)
	}
	if urls[0] != "https://example.com/ref.mp4?token=1" {
		t.Fatalf("url = %q, want reference video url", urls[0])
	}
}

func TestExtractReferenceVideoURLsDetectsVideoExtensionInImageURL(t *testing.T) {
	body := []byte(`{
		"content": [
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://example.com/not-video.mp4"}}
		]
	}`)

	urls := ExtractReferenceVideoURLsFromBody(body)
	if len(urls) != 1 {
		t.Fatalf("len(urls) = %d, want 1: %#v", len(urls), urls)
	}
	if urls[0] != "https://example.com/not-video.mp4" {
		t.Fatalf("url = %q, want video url from image_url field", urls[0])
	}
}

func TestExtractReferenceVideoURLsDoesNotTreatImageAsVideo(t *testing.T) {
	body := []byte(`{
		"content": [
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://example.com/not-video.png"}}
		]
	}`)

	if urls := ExtractReferenceVideoURLsFromBody(body); len(urls) != 0 {
		t.Fatalf("urls = %#v, want none", urls)
	}
}

func TestExtractReferenceVideoURLsSupportsReferenceVideoRole(t *testing.T) {
	body := []byte(`{
		"content": [
			{"type":"input","role":"reference_video","url":"data:video/mp4;base64,AAAA"}
		]
	}`)

	urls := ExtractReferenceVideoURLsFromBody(body)
	if len(urls) != 1 {
		t.Fatalf("len(urls) = %d, want 1: %#v", len(urls), urls)
	}
}

func TestExtractReferenceVideoURLsAcceptsMarkedURLWithoutExtension(t *testing.T) {
	body := []byte(`{
		"content": [
			{"type":"video_url","role":"reference_video","video_url":{"url":"https://cdn.example.com/signed?id=abc"}}
		]
	}`)

	urls := ExtractReferenceVideoURLsFromBody(body)
	if len(urls) != 1 {
		t.Fatalf("len(urls) = %d, want 1: %#v", len(urls), urls)
	}
	if urls[0] != "https://cdn.example.com/signed?id=abc" {
		t.Fatalf("url = %q, want signed url", urls[0])
	}
}

func TestExtractReferenceVideoURLsNormalizesBareBase64(t *testing.T) {
	body := []byte(`{
		"content": [
			{"type":"video_url","role":"reference_video","video_url":{"base64":"AAAAIGZ0eXBpc29tAAACAGlzb21pc28yYXZjMW1wNDE="}}
		]
	}`)

	urls := ExtractReferenceVideoURLsFromBody(body)
	if len(urls) != 1 {
		t.Fatalf("len(urls) = %d, want 1: %#v", len(urls), urls)
	}
	if urls[0] != "data:video/mp4;base64,AAAAIGZ0eXBpc29tAAACAGlzb21pc28yYXZjMW1wNDE=" {
		t.Fatalf("url = %q, want normalized data video url", urls[0])
	}
}
