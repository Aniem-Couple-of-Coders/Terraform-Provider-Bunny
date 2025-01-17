package provider

import bunny "github.com/Aniem-Couple-of-Coders/Go-Module-Bunny"

var edgeRuleTriggerTypesStr = map[string]int{
	"url":             bunny.EdgeRuleTriggerTypeURL,
	"request_header":  bunny.EdgeRuleTriggerTypeRequestHeader,
	"response_header": bunny.EdgeRuleTriggerTypeResponseHeader,
	"url_extensions":  bunny.EdgeRuleTriggerTypeURLExtension,
	"country_code":    bunny.EdgeRuleTriggerTypeCountryCode,
	"remote_ip":       bunny.EdgeRuleTriggerTypeRemoteIP,
	"query_string":    bunny.EdgeRuleTriggerTypeURLQueryString,
	"random_chance":   bunny.EdgeRuleTriggerTypeRandomChance,
	"status_code":     bunny.EdgeRuleTriggerTypeStatusCode,
	"request_method":  bunny.EdgeRuleTriggerTypeRequestMethod,
}

var edgeRuleTriggerTypesInt = reverseStrIntMap(edgeRuleTriggerTypesStr)

var edgeRuleTriggerTypeKeys = strIntMapKeysSorted(edgeRuleTriggerTypesStr)
