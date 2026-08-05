package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
)

const ginKeyUserChannelRoutingDecisions = "user_channel_routing_decisions"
const ginKeyUserChannelRoutingLogInfo = "user_channel_routing_log_info"

type userChannelRoutingDecision struct {
	Match   model_setting.UserChannelRoutingMatch
	Matched bool
}

func resolveUserChannelRouting(c *gin.Context, usingGroup, modelName string) userChannelRoutingDecision {
	cacheKey := usingGroup + "\x00" + modelName
	if value, exists := c.Get(ginKeyUserChannelRoutingDecisions); exists {
		if decisions, ok := value.(map[string]userChannelRoutingDecision); ok {
			if decision, found := decisions[cacheKey]; found {
				if decision.Matched {
					setUserChannelRoutingLogInfo(c, decision, usingGroup, modelName, 0, false, "")
				} else {
					c.Set(ginKeyUserChannelRoutingLogInfo, nil)
				}
				return decision
			}
		}
	}

	match, matched := model_setting.MatchUserChannelRouting(
		common.GetContextKeyInt(c, constant.ContextKeyUserId),
		usingGroup,
		modelName,
	)
	decision := userChannelRoutingDecision{Match: match, Matched: matched}
	decisions := make(map[string]userChannelRoutingDecision)
	if value, exists := c.Get(ginKeyUserChannelRoutingDecisions); exists {
		if existing, ok := value.(map[string]userChannelRoutingDecision); ok {
			for key, cached := range existing {
				decisions[key] = cached
			}
		}
	}
	decisions[cacheKey] = decision
	c.Set(ginKeyUserChannelRoutingDecisions, decisions)
	if matched {
		setUserChannelRoutingLogInfo(c, decision, usingGroup, modelName, 0, false, "")
	} else {
		c.Set(ginKeyUserChannelRoutingLogInfo, nil)
	}
	return decision
}

func userChannelRoutingAllows(decision userChannelRoutingDecision, channelID int) bool {
	if !decision.Matched {
		return true
	}
	_, allowed := decision.Match.AllowedChannelIDs[channelID]
	return allowed
}

func AllowSelectedChannelByUserRouting(c *gin.Context, usingGroup, modelName string, channel *model.Channel) bool {
	if c == nil || channel == nil {
		return false
	}
	if usingGroup == "auto" {
		userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
		matchedAnyRule := false
		for _, actualGroup := range GetUserAutoGroup(userGroup) {
			decision := resolveUserChannelRouting(c, actualGroup, modelName)
			matchedAnyRule = matchedAnyRule || decision.Matched
			if !model.IsChannelEnabledForGroupModel(actualGroup, modelName, channel.Id) {
				continue
			}
			if !userChannelRoutingAllows(decision, channel.Id) {
				continue
			}
			common.SetContextKey(c, constant.ContextKeyAutoGroup, actualGroup)
			if decision.Matched {
				setUserChannelRoutingLogInfo(c, decision, actualGroup, modelName, channel.Id, false, "")
			}
			return true
		}
		return !matchedAnyRule
	}
	decision := resolveUserChannelRouting(c, usingGroup, modelName)
	allowed := userChannelRoutingAllows(decision, channel.Id)
	if decision.Matched && allowed {
		setUserChannelRoutingLogInfo(c, decision, usingGroup, modelName, channel.Id, false, "")
	}
	return allowed
}

func setUserChannelRoutingLogInfo(c *gin.Context, decision userChannelRoutingDecision, usingGroup, modelName string, selectedChannelID int, fallbackUsed bool, fallbackReason string) {
	if c == nil || !decision.Matched {
		return
	}
	info := map[string]interface{}{
		"rule_id":             decision.Match.Rule.ID,
		"rule_name":           decision.Match.Rule.Name,
		"using_group":         usingGroup,
		"routed_model":        modelName,
		"channel_ids":         append([]int(nil), decision.Match.Rule.ChannelIDs...),
		"fallback_mode":       decision.Match.Rule.Fallback,
		"fallback_used":       fallbackUsed,
		"selected_channel_id": selectedChannelID,
	}
	if requestedModel := common.GetContextKeyString(c, constant.ContextKeyOriginalModel); requestedModel != "" {
		info["requested_model"] = requestedModel
	}
	if fallbackReason != "" {
		info["fallback_reason"] = fallbackReason
	}
	c.Set(ginKeyUserChannelRoutingLogInfo, info)
}

func AppendUserChannelRoutingAdminInfo(c *gin.Context, adminInfo map[string]interface{}) {
	if c == nil || adminInfo == nil {
		return
	}
	value, exists := c.Get(ginKeyUserChannelRoutingLogInfo)
	if !exists || value == nil {
		return
	}
	info, ok := value.(map[string]interface{})
	if !ok {
		return
	}
	logInfo := make(map[string]interface{}, len(info)+1)
	for key, field := range info {
		logInfo[key] = field
	}
	if requestedModel := common.GetContextKeyString(c, constant.ContextKeyOriginalModel); requestedModel != "" {
		logInfo["requested_model"] = requestedModel
	}
	adminInfo["user_channel_routing"] = logInfo
}

func userChannelRoutingCandidateFilter(decision userChannelRoutingDecision) func(*model.Channel) bool {
	if !decision.Matched {
		return nil
	}
	return func(channel *model.Channel) bool {
		return channel != nil && userChannelRoutingAllows(decision, channel.Id)
	}
}
