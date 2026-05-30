package operation_setting

import (
	"strings"
	"sync/atomic"
)

var DemoSiteEnabled = false
var SelfUseModeEnabled = false

var defaultAutomaticDisableKeywords = []string{
	"Your credit balance is too low",
	"This organization has been disabled.",
	"You exceeded your current quota",
	"Permission denied",
	"The security token included in the request is invalid",
	"Operation not allowed",
	"Your account is not authorized",
}

var automaticDisableKeywords atomic.Value

func init() {
	SetAutomaticDisableKeywords(defaultAutomaticDisableKeywords)
}

func GetAutomaticDisableKeywords() []string {
	keywords, ok := automaticDisableKeywords.Load().([]string)
	if !ok {
		return nil
	}
	return append([]string(nil), keywords...)
}

func SetAutomaticDisableKeywords(keywords []string) {
	normalized := make([]string, 0, len(keywords))
	for _, k := range keywords {
		k = strings.TrimSpace(k)
		k = strings.ToLower(k)
		if k != "" {
			normalized = append(normalized, k)
		}
	}
	automaticDisableKeywords.Store(normalized)
}

func AutomaticDisableKeywordsToString() string {
	return strings.Join(GetAutomaticDisableKeywords(), "\n")
}

func AutomaticDisableKeywordsFromString(s string) {
	ak := strings.Split(s, "\n")
	SetAutomaticDisableKeywords(ak)
}
