//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func openAIVideoTestAccount() *Account {
	return &Account{
		ID:          91,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "upstream-key",
			"base_url": "https://video-upstream.example/v1",
			"model_mapping": map[string]any{
				"3.0": "upstream-video-3",
			},
		},
	}
}

func openAIVideoResponse(status int, contentType, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{contentType}, "X-Request-Id": []string{"upstream-request"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestOpenAIVideoCapabilityAllowsOnlyOpenAIAPIKeyAccounts(t *testing.T) {
	require.True(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}).SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityVideos))
	require.False(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}).SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityVideos))
	require.False(t, (&Account{Platform: PlatformGrok, Type: AccountTypeAPIKey}).SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityVideos))
}

func TestParseGrokMediaRequestRecognizesOpenAIVideoAliases(t *testing.T) {
	info := ParseGrokMediaRequest("application/json", []byte(`{"model":"3.0","seconds":6,"resolution_name":"720p"}`))

	require.Equal(t, 6, info.DurationSeconds)
	require.Equal(t, VideoBillingResolution720P, info.Resolution)
}

func TestBuildOpenAIVideoURLHandlesVersionedBaseURL(t *testing.T) {
	require.Equal(t, "https://video-upstream.example/v1/videos", buildOpenAIVideoURL("https://video-upstream.example/v1", GrokMediaEndpointVideosGenerations, ""))
	require.Equal(t, "https://video-upstream.example/v1/videos/task%2Fone", buildOpenAIVideoURL("https://video-upstream.example/v1", GrokMediaEndpointVideoStatus, "task/one"))
	require.Equal(t, "https://video-upstream.example/v1/videos/task%2Fone/content", buildOpenAIVideoURL("https://video-upstream.example/v1", GrokMediaEndpointVideoContent, "task/one"))
}

func TestForwardOpenAIVideoCreateAppliesChannelAndAccountModelMappings(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{response: openAIVideoResponse(http.StatusOK, "application/json", `{"data":{"task_id":"task-1","status":"queued"}}`)}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, recorder := grokMediaContentTestContext(http.MethodPost, "https://api.example/v1/videos", nil)
	body := []byte(`{"model":"video3.0","prompt":"waves","seconds":6,"resolution_name":"720p"}`)

	result, err := svc.ForwardOpenAIVideo(context.Background(), c, openAIVideoTestAccount(), GrokMediaEndpointVideosGenerations, "", body, "application/json", "3.0")

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "task-1", result.ResponseID)
	require.Equal(t, "3.0", result.Model)
	require.Equal(t, "upstream-video-3", result.UpstreamModel)
	require.Equal(t, 6, result.VideoDurationSeconds)
	require.Equal(t, VideoBillingResolution720P, result.VideoResolution)
	require.Equal(t, http.MethodPost, upstream.request.Method)
	require.Equal(t, "https://video-upstream.example/v1/videos", upstream.request.URL.String())
	require.Equal(t, "Bearer upstream-key", upstream.request.Header.Get("Authorization"))
	forwarded, readErr := io.ReadAll(upstream.request.Body)
	require.NoError(t, readErr)
	require.Equal(t, "upstream-video-3", gjson.GetBytes(forwarded, "model").String())
	require.Equal(t, "waves", gjson.GetBytes(forwarded, "prompt").String())
	require.Equal(t, int64(6), gjson.GetBytes(forwarded, "seconds").Int())
	require.Equal(t, "720p", gjson.GetBytes(forwarded, "resolution_name").String())
}

func TestForwardOpenAIVideoCreateRewritesMultipartModelWithoutDroppingParts(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "video3.0"))
	require.NoError(t, writer.WriteField("prompt", "ocean"))
	require.NoError(t, writer.WriteField("seconds", "5"))
	require.NoError(t, writer.WriteField("resolution_name", "1080p"))
	require.NoError(t, writer.WriteField("preset", "cinematic"))
	file, err := writer.CreateFormFile("first_frame", "frame.png")
	require.NoError(t, err)
	_, err = file.Write([]byte("png-payload"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	upstream := &grokMediaContentUpstreamStub{response: openAIVideoResponse(http.StatusOK, "application/json", `{"id":"task-2","status":"queued"}`)}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, _ := grokMediaContentTestContext(http.MethodPost, "https://api.example/v1/videos", nil)

	result, err := svc.ForwardOpenAIVideo(context.Background(), c, openAIVideoTestAccount(), GrokMediaEndpointVideosGenerations, "", body.Bytes(), writer.FormDataContentType(), "3.0")

	require.NoError(t, err)
	require.Equal(t, "task-2", result.ResponseID)
	require.Equal(t, 5, result.VideoDurationSeconds)
	require.Equal(t, VideoBillingResolution1080P, result.VideoResolution)
	mediaType, params, parseErr := mime.ParseMediaType(upstream.request.Header.Get("Content-Type"))
	require.NoError(t, parseErr)
	require.Equal(t, "multipart/form-data", mediaType)
	forwarded, readErr := io.ReadAll(upstream.request.Body)
	require.NoError(t, readErr)
	reader := multipart.NewReader(bytes.NewReader(forwarded), params["boundary"])
	fields := map[string]string{}
	files := map[string][]byte{}
	for {
		part, nextErr := reader.NextPart()
		if nextErr == io.EOF {
			break
		}
		require.NoError(t, nextErr)
		data, partErr := io.ReadAll(part)
		require.NoError(t, partErr)
		if part.FileName() == "" {
			fields[part.FormName()] = string(data)
		} else {
			files[part.FormName()] = data
		}
	}
	require.Equal(t, "upstream-video-3", fields["model"])
	require.Equal(t, "ocean", fields["prompt"])
	require.Equal(t, "5", fields["seconds"])
	require.Equal(t, "1080p", fields["resolution_name"])
	require.Equal(t, "cinematic", fields["preset"])
	require.Equal(t, []byte("png-payload"), files["first_frame"])
}

func TestForwardOpenAIVideoStatusUsesCanonicalGETAndExtractsCompletion(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{response: openAIVideoResponse(http.StatusOK, "application/json", `{"data":{"task_id":"task-3","state":"completed","output_url":"https://cdn.example/task-3.mp4","model":"3.0","seconds":7}}`)}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, recorder := grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/task-3", nil)

	result, err := svc.ForwardOpenAIVideo(context.Background(), c, openAIVideoTestAccount(), GrokMediaEndpointVideoStatus, "task-3", nil, "", "")

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, http.MethodGet, upstream.request.Method)
	require.Equal(t, "https://video-upstream.example/v1/videos/task-3", upstream.request.URL.String())
	require.Equal(t, "task-3", result.ResponseID)
	require.Equal(t, 1, result.VideoCount)
	require.Equal(t, 7, result.VideoDurationSeconds)
}

func TestForwardOpenAIVideoStatusPreservesDirectVideoURL(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{response: openAIVideoResponse(http.StatusOK, "application/json", `{"id":"task-direct","status":"completed","video":{"url":"https://cdn.example/task-direct.mp4"}}`)}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, recorder := grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/task-direct", nil)

	_, err := svc.ForwardOpenAIVideo(context.Background(), c, openAIVideoTestAccount(), GrokMediaEndpointVideoStatus, "task-direct", nil, "", "")

	require.NoError(t, err)
	require.Equal(t, "https://cdn.example/task-direct.mp4", gjson.Get(recorder.Body.String(), "video.url").String())
}

func TestForwardOpenAIVideoPreservesUpstreamErrorStatusAndBody(t *testing.T) {
	want := `{"error":{"message":"invalid video parameters","type":"invalid_request_error"}}`
	upstream := &grokMediaContentUpstreamStub{response: openAIVideoResponse(http.StatusBadRequest, "application/json", want)}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, recorder := grokMediaContentTestContext(http.MethodPost, "https://api.example/v1/videos", nil)

	_, err := svc.ForwardOpenAIVideo(context.Background(), c, openAIVideoTestAccount(), GrokMediaEndpointVideosGenerations, "", []byte(`{"model":"video3.0"}`), "application/json", "3.0")

	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.JSONEq(t, want, recorder.Body.String())
}

func TestForwardOpenAIVideoContentStreamsRangeAndMarksCompletion(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{response: &http.Response{
		StatusCode: http.StatusPartialContent,
		Header: http.Header{
			"Content-Type":  []string{"video/mp4"},
			"Content-Range": []string{"bytes 0-4/10"},
		},
		Body: io.NopCloser(strings.NewReader("video")),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, recorder := grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/task-4/content", map[string]string{"Range": "bytes=0-4"})

	result, err := svc.ForwardOpenAIVideo(context.Background(), c, openAIVideoTestAccount(), GrokMediaEndpointVideoContent, "task-4", nil, "", "")

	require.NoError(t, err)
	require.Equal(t, http.StatusPartialContent, recorder.Code)
	require.Equal(t, "video", recorder.Body.String())
	require.Equal(t, "https://video-upstream.example/v1/videos/task-4/content", upstream.request.URL.String())
	require.Equal(t, "bytes=0-4", upstream.request.Header.Get("Range"))
	require.Equal(t, 1, result.VideoCount)
	require.Equal(t, "task-4", result.ResponseID)
}
