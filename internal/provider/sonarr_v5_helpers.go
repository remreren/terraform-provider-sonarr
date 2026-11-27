package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Helpers for fields that only some Sonarr versions have (v5+ added several to API v3). Sonarr v5
// resets a field that a PUT leaves out, and crashes on some (500), so a field the configuration
// doesn't set is filled from Sonarr's current value before writing; v4 doesn't send these fields,
// so they stay unset there.

// knownValue is a framework value that is configured (neither null nor unknown).
type knownValue interface {
	IsNull() bool
	IsUnknown() bool
}

func isKnown(v knownValue) bool {
	return !v.IsNull() && !v.IsUnknown()
}

// setIfKnown sends a string attribute only when the configuration sets it.
func setIfKnown(v types.String, set func(string)) {
	if isKnown(v) {
		set(v.ValueString())
	}
}

func setBoolIfKnown(v types.Bool, set func(bool)) {
	if isKnown(v) {
		set(v.ValueBool())
	}
}

func setInt32IfKnown(v types.Int64, set func(int32)) {
	if isKnown(v) {
		set(int32(v.ValueInt64()))
	}
}

func setFloatIfKnown(v types.Float64, set func(float64)) {
	if isKnown(v) {
		set(v.ValueFloat64())
	}
}

// keepOrRead is Sonarr's value when it has the field; else the previous one (Sonarr v4: the field
// doesn't exist), null when there is none.
func keepOrRead(prev types.String, get func() (*string, bool)) types.String {
	if v, ok := get(); ok {
		return types.StringValue(derefString(v))
	}

	return keepOrNull(prev, types.StringNull()).(types.String)
}

func keepOrReadBool(prev types.Bool, get func() (*bool, bool)) types.Bool {
	if v, ok := get(); ok && v != nil {
		return types.BoolValue(*v)
	}

	return keepOrNull(prev, types.BoolNull()).(types.Bool)
}

func keepOrReadInt32(prev types.Int64, get func() (*int32, bool)) types.Int64 {
	if v, ok := get(); ok && v != nil {
		return types.Int64Value(int64(*v))
	}

	return keepOrNull(prev, types.Int64Null()).(types.Int64)
}

func keepOrReadFloat(prev types.Float64, get func() (*float64, bool)) types.Float64 {
	if v, ok := get(); ok && v != nil {
		return types.Float64Value(*v)
	}

	return keepOrNull(prev, types.Float64Null()).(types.Float64)
}

func keepOrNull(prev attr.Value, null attr.Value) attr.Value {
	if prev == nil || prev.IsUnknown() {
		return null
	}

	return prev
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}

	return *v
}
