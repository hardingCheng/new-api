package common

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestRelayInfoGetFinalRequestRelayFormatPrefersExplicitFinal(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:             types.RelayFormatOpenAI,
		RequestConversionChain:  []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
		FinalRequestRelayFormat: types.RelayFormatOpenAIResponses,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatFallsBackToConversionChain(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:            types.RelayFormatOpenAI,
		RequestConversionChain: []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatClaude), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatFallsBackToRelayFormat(t *testing.T) {
	info := &RelayInfo{
		RelayFormat: types.RelayFormatGemini,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatGemini), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatNilReceiver(t *testing.T) {
	var info *RelayInfo
	require.Equal(t, types.RelayFormat(""), info.GetFinalRequestRelayFormat())
}

func TestTaskSubmitReqUnmarshalSecondsStringOrNumber(t *testing.T) {
	var stringReq TaskSubmitReq
	err := common.Unmarshal([]byte(`{"prompt":"p","model":"m","seconds":"5"}`), &stringReq)
	require.NoError(t, err)
	require.Equal(t, "5", stringReq.Seconds)

	var numberReq TaskSubmitReq
	err = common.Unmarshal([]byte(`{"prompt":"p","model":"m","seconds":5}`), &numberReq)
	require.NoError(t, err)
	require.Equal(t, "5", numberReq.Seconds)
}

func TestTaskSubmitReqUnmarshalInputReferenceStringOrArray(t *testing.T) {
	var stringReq TaskSubmitReq
	err := common.Unmarshal([]byte(`{"prompt":"p","model":"m","input_reference":"https://example.com/a.png"}`), &stringReq)
	require.NoError(t, err)
	require.Equal(t, "https://example.com/a.png", stringReq.InputReference)
	require.Equal(t, []string{"https://example.com/a.png"}, stringReq.InputReferenceValues())

	var arrayReq TaskSubmitReq
	err = common.Unmarshal([]byte(`{"prompt":"p","model":"m","input_reference":["https://example.com/a.png","https://example.com/b.png"]}`), &arrayReq)
	require.NoError(t, err)
	require.Equal(t, "https://example.com/a.png", arrayReq.InputReference)
	require.Equal(t, []string{"https://example.com/a.png", "https://example.com/b.png"}, arrayReq.InputReferenceValues())
	require.Equal(t, []string{"https://example.com/a.png", "https://example.com/b.png"}, arrayReq.InputReferences)
}

func TestFillMissingGrokImagineInputReference(t *testing.T) {
	req := TaskSubmitReq{
		Model:  "grok-imagine-video",
		Image:  "https://example.com/a.png",
		Images: []string{"https://example.com/b.png"},
	}

	FillMissingGrokImagineInputReference(nil, &req)

	require.Equal(t, "https://example.com/a.png", req.InputReference)
	require.Equal(t, []string{"https://example.com/a.png", "https://example.com/b.png"}, req.InputReferenceValues())
}
