package provider

import (
	bunny "github.com/Aniem-Couple-of-Coders/Go-Module-Bunny"
)

var edgeRuleMatchingTypesStr = map[string]int{
	"any":  bunny.MatchingTypeAny,
	"all":  bunny.MatchingTypeAll,
	"none": bunny.MatchingTypeNone,
}

var edgeRuleMatchingTypesInt = reverseStrIntMap(edgeRuleMatchingTypesStr)

var edgeRuleMatchingTypeKeys = strIntMapKeysSorted(edgeRuleMatchingTypesStr)
