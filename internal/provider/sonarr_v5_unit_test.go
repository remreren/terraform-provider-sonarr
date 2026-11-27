package provider

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/devopsarr/sonarr-go/sonarr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Unit tests (no Sonarr needed) for the Sonarr v5 changes:
// - media management: userRejectedExtensions is always sent (v5 answers 500 without it);
// - quality definitions: v5 has no sizes there; keep the configured ones and warn;
// - quality profiles: v5 keeps the sizes per quality on the profile items.

func mustUnmarshal[T any](t *testing.T, src string) *T {
	t.Helper()

	var v T
	if err := json.Unmarshal([]byte(src), &v); err != nil {
		t.Fatal(err)
	}

	return &v
}

func TestMediaManagementAlwaysSendsUserRejectedExtensions(t *testing.T) {
	t.Parallel()

	m := MediaManagement{UserRejectedExtensions: types.StringValue("")}
	out, _ := json.Marshal(m.read())

	if !strings.Contains(string(out), `"userRejectedExtensions":""`) {
		t.Fatalf("v5 needs the field even when empty: %s", out)
	}
}

func TestMediaManagementWriteV4KeepsPlannedUserRejectedExtensions(t *testing.T) {
	t.Parallel()

	v4 := mustUnmarshal[sonarr.MediaManagementConfigResource](t, `{"id":1,"extraFileExtensions":"srt"}`)
	m := MediaManagement{UserRejectedExtensions: types.StringValue("exe")}
	m.write(v4)

	if m.UserRejectedExtensions.ValueString() != "exe" {
		t.Fatalf("v4 answer without the field must keep the plan, got %q", m.UserRejectedExtensions.ValueString())
	}
}

func TestMediaManagementWriteV5ReadsUserRejectedExtensions(t *testing.T) {
	t.Parallel()

	v5 := mustUnmarshal[sonarr.MediaManagementConfigResource](t, `{"id":1,"userRejectedExtensions":"exe,lnk"}`)
	m := MediaManagement{UserRejectedExtensions: types.StringValue("")}
	m.write(v5)

	if m.UserRejectedExtensions.ValueString() != "exe,lnk" {
		t.Fatalf("got %q", m.UserRejectedExtensions.ValueString())
	}
}

func TestQualityDefinitionWriteV5KeepsConfiguredSizes(t *testing.T) {
	t.Parallel()

	v5 := mustUnmarshal[sonarr.QualityDefinitionResource](t, `{"id":4,"title":"WEBDL-480p","weight":3,"quality":{"id":8,"name":"WEBDL-480p"}}`)
	p := QualityDefinition{MinSize: types.Float64Value(4), MaxSize: types.Float64Value(85), PreferredSize: types.Float64Value(60)}
	p.write(v5)

	if p.MinSize.ValueFloat64() != 4 || p.MaxSize.ValueFloat64() != 85 || p.PreferredSize.ValueFloat64() != 60 {
		t.Fatalf("sizes lost: %v %v %v", p.MinSize, p.MaxSize, p.PreferredSize)
	}

	var diags diag.Diagnostics
	sizesIgnoredWarning(v5, &diags)

	if diags.WarningsCount() != 1 || diags.HasError() {
		t.Fatalf("want one warning, got %v", diags)
	}
}

func TestQualityDefinitionWriteV5ImportIsNull(t *testing.T) {
	t.Parallel()

	v5 := mustUnmarshal[sonarr.QualityDefinitionResource](t, `{"id":4,"title":"WEBDL-480p","quality":{"id":8}}`)
	p := QualityDefinition{MinSize: types.Float64Unknown(), MaxSize: types.Float64Null(), PreferredSize: types.Float64Unknown()}
	p.write(v5)

	if !p.MinSize.IsNull() || !p.MaxSize.IsNull() || !p.PreferredSize.IsNull() {
		t.Fatalf("no sizes from Sonarr and none configured: null, got %v %v %v", p.MinSize, p.MaxSize, p.PreferredSize)
	}
}

func TestQualityDefinitionWriteV4ReadsSizes(t *testing.T) {
	t.Parallel()

	v4 := mustUnmarshal[sonarr.QualityDefinitionResource](t, `{"id":4,"title":"WEBDL-480p","minSize":2,"maxSize":100,"preferredSize":95,"quality":{"id":8}}`)
	p := QualityDefinition{MinSize: types.Float64Value(4), MaxSize: types.Float64Value(85), PreferredSize: types.Float64Value(60)}
	p.write(v4)

	if p.MinSize.ValueFloat64() != 2 || p.MaxSize.ValueFloat64() != 100 || p.PreferredSize.ValueFloat64() != 95 {
		t.Fatalf("v4 sizes must come from Sonarr (drift shows): %v %v %v", p.MinSize, p.MaxSize, p.PreferredSize)
	}

	var diags diag.Diagnostics
	sizesIgnoredWarning(v4, &diags)

	if diags.WarningsCount() != 0 {
		t.Fatalf("no warning on v4, got %v", diags)
	}
}

func TestQualityGroupWriteReadsSizesPerQuality(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// v5: a single quality carries its sizes on the item, a group on its sub-items (the group: null).
	single := mustUnmarshal[sonarr.QualityProfileQualityItemResource](t,
		`{"quality":{"id":1,"name":"SDTV","source":"television","resolution":480},"items":[],"allowed":true,"minSize":2,"maxSize":100,"preferredSize":95}`)
	group := mustUnmarshal[sonarr.QualityProfileQualityItemResource](t,
		`{"id":1001,"name":"WEB 1080p","allowed":true,"minSize":null,"items":[
		  {"quality":{"id":3,"name":"WEBDL-1080p","source":"web","resolution":1080},"items":[],"allowed":true,"minSize":4,"maxSize":85,"preferredSize":60},
		  {"quality":{"id":15,"name":"WEBRip-1080p","source":"webRip","resolution":1080},"items":[],"allowed":true}]}`)

	var diags diag.Diagnostics

	g1 := QualityGroup{}
	g1.write(ctx, single, &diags)

	q1 := make([]ProfileQuality, 0)
	diags.Append(g1.Qualities.ElementsAs(ctx, &q1, false)...)

	if len(q1) != 1 || q1[0].MinSize.ValueFloat64() != 2 || q1[0].MaxSize.ValueFloat64() != 100 || q1[0].PreferredSize.ValueFloat64() != 95 {
		t.Fatalf("single quality sizes: %+v", q1)
	}

	g2 := QualityGroup{}
	g2.write(ctx, group, &diags)

	q2 := make([]ProfileQuality, 0)
	diags.Append(g2.Qualities.ElementsAs(ctx, &q2, false)...)

	if len(q2) != 2 || q2[0].MinSize.ValueFloat64() != 4 || q2[0].MaxSize.ValueFloat64() != 85 || q2[0].PreferredSize.ValueFloat64() != 60 {
		t.Fatalf("grouped quality sizes: %+v", q2)
	}

	if !q2[1].MinSize.IsNull() || !q2[1].MaxSize.IsNull() || !q2[1].PreferredSize.IsNull() {
		t.Fatalf("no sizes (v4): null, got %+v", q2[1])
	}

	if diags.HasError() {
		t.Fatal(diags)
	}
}

func TestQualityGroupReadSendsOnlyConfiguredSizes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	var diags diag.Diagnostics

	qualities, d := types.ListValueFrom(ctx, ProfileQuality{}.getType(), []ProfileQuality{
		{
			ID: types.Int64Value(3), Name: types.StringValue("WEBDL-1080p"), Source: types.StringValue("web"), Resolution: types.Int64Value(1080),
			MinSize: types.Float64Value(4), MaxSize: types.Float64Value(85), PreferredSize: types.Float64Value(60),
		},
		{
			ID: types.Int64Value(15), Name: types.StringValue("WEBRip-1080p"), Source: types.StringValue("webRip"), Resolution: types.Int64Value(1080),
			MinSize: types.Float64Null(), MaxSize: types.Float64Unknown(), PreferredSize: types.Float64Null(),
		},
	})
	diags.Append(d...)

	g := QualityGroup{ID: types.Int64Value(1001), Name: types.StringValue("WEB 1080p"), Qualities: qualities}

	var allowed []int32

	item := g.read(ctx, &allowed, &diags)
	out, _ := json.Marshal(item)

	for _, want := range []string{`"minSize":4`, `"maxSize":85`, `"preferredSize":60`} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("%s not sent: %s", want, out)
		}
	}

	if strings.Count(string(out), "Size") != 3 {
		t.Fatalf("null/unknown sizes must not be sent (v4, or sizes left to Sonarr): %s", out)
	}

	if diags.HasError() {
		t.Fatal(diags)
	}
}

func TestFillHostFromCurrentV5(t *testing.T) {
	t.Parallel()

	current := mustUnmarshal[sonarr.HostConfigResource](t, `{"id":1,"allowedHosts":"a","trustedNetworks":"b",
		"oidcAuthority":"https://id","oidcClientId":"c","oidcClientSecret":"s","oidcScopes":"openid","oidcUserIdentifier":"","sslKeyPath":"/k"}`)
	request := sonarr.NewHostConfigResource()
	request.SetOidcClientId("mine")
	fillHostFromCurrent(request, current)

	out, _ := json.Marshal(request)
	for _, want := range []string{`"oidcAuthority":"https://id"`, `"oidcClientId":"mine"`, `"oidcClientSecret":"s"`, `"oidcScopes":"openid"`, `"oidcUserIdentifier":""`, `"sslKeyPath":"/k"`, `"allowedHosts":"a"`} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("%s missing (v5 answers 500 without it): %s", want, out)
		}
	}
}

func TestFillHostFromCurrentV4SendsNoOidc(t *testing.T) {
	t.Parallel()

	current := mustUnmarshal[sonarr.HostConfigResource](t, `{"id":1,"allowedHosts":"a","trustedNetworks":"b"}`)
	request := sonarr.NewHostConfigResource()
	fillHostFromCurrent(request, current)

	out, _ := json.Marshal(request)
	if strings.Contains(string(out), "oidc") || strings.Contains(string(out), "sslKeyPath") {
		t.Fatalf("v4 has no OIDC fields: %s", out)
	}
}

func TestFillMediaManagementFromCurrentKeepsUnmanaged(t *testing.T) {
	t.Parallel()

	current := mustUnmarshal[sonarr.MediaManagementConfigResource](t, `{"id":1,"userRejectedExtensions":"exe","seasonPackUpgrade":"threshold","seasonPackUpgradeThreshold":75,"skipFreeSpaceCheckWhenGrabbing":true}`)
	m := MediaManagement{
		UserRejectedExtensions: types.StringUnknown(), SeasonPackUpgrade: types.StringValue("any"),
		SeasonPackThreshold: types.Float64Unknown(), SkipFreeSpaceCheckGrab: types.BoolNull(),
	}
	request := m.read()
	fillMediaManagementFromCurrent(request, current)

	if request.GetUserRejectedExtensions() != "exe" || request.GetSeasonPackUpgradeThreshold() != 75 || !request.GetSkipFreeSpaceCheckWhenGrabbing() {
		t.Fatalf("unmanaged fields must keep Sonarr's values (v5 resets them otherwise): %+v", request)
	}

	if request.GetSeasonPackUpgrade() != sonarr.SEASONPACKUPGRADETYPE_ANY {
		t.Fatalf("configured value must win, got %v", request.GetSeasonPackUpgrade())
	}
}

func TestReleaseProfileV5FieldsReadWriteAndKeep(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	var diags diag.Diagnostics

	excluded, _ := types.SetValueFrom(ctx, types.Int64Type, []int64{4})
	p := ReleaseProfile{
		Tags: types.SetNull(types.Int64Type), Required: types.SetNull(types.StringType), Ignored: types.SetNull(types.StringType),
		AirDateRestriction: types.BoolValue(true), AirDateGracePeriod: types.Int64Unknown(),
		AllowSeasonPackNotAired: types.BoolNull(), ExcludedTags: excluded,
	}
	request := p.read(ctx, &diags)

	current := mustUnmarshal[sonarr.ReleaseProfileResource](t, `{"id":1,"airDateRestriction":false,"airDateGracePeriod":3,"allowSeasonPackWithoutAllEpisodesAired":true,"excludedTags":[9]}`)
	fillReleaseProfileFromCurrent(request, current)

	if !request.GetAirDateRestriction() || request.GetAirDateGracePeriod() != 3 || !request.GetAllowSeasonPackWithoutAllEpisodesAired() || len(request.GetExcludedTags()) != 1 || request.GetExcludedTags()[0] != 4 {
		t.Fatalf("configured values win, unmanaged ones keep Sonarr's: %+v", request)
	}

	v4 := mustUnmarshal[sonarr.ReleaseProfileResource](t, `{"id":1,"name":"x","enabled":true}`)
	p.write(ctx, v4, &diags)

	if !p.AirDateRestriction.ValueBool() || !p.AirDateGracePeriod.IsNull() || p.ExcludedTags.IsNull() {
		t.Fatalf("v4 answer: keep the plan, unknown becomes null: %+v", p)
	}

	if diags.HasError() {
		t.Fatal(diags)
	}
}

func TestReleaseProfileWriteV4DataSourceTypedNull(t *testing.T) {
	t.Parallel()

	var diags diag.Diagnostics

	p := ReleaseProfile{} // a data source: no plan, zero values
	p.write(context.Background(), mustUnmarshal[sonarr.ReleaseProfileResource](t, `{"id":1,"name":"x"}`), &diags)

	if !p.ExcludedTags.IsNull() || p.ExcludedTags.ElementType(context.Background()) == nil {
		t.Fatalf("v4: a typed null set, got %#v", p.ExcludedTags)
	}
}

func TestFillImportListAndSeriesFromCurrent(t *testing.T) {
	t.Parallel()

	list := sonarr.NewImportListResource()
	fillImportListFromCurrent(list, mustUnmarshal[sonarr.ImportListResource](t, `{"id":1,"language":{"id":2,"name":"French"},"tagExisting":"sync"}`))

	if !list.HasLanguage() || list.GetTagExisting() != sonarr.TAGEXISTINGTYPE_SYNC {
		t.Fatalf("v5 import list fields must be kept: %+v", list)
	}

	empty := sonarr.NewImportListResource()
	fillImportListFromCurrent(empty, mustUnmarshal[sonarr.ImportListResource](t, `{"id":1}`))

	if empty.HasLanguage() || empty.HasTagExisting() {
		t.Fatal("v4: nothing to copy")
	}

	series := sonarr.NewSeriesResource()
	fillSeriesFromCurrent(series, mustUnmarshal[sonarr.SeriesResource](t, `{"id":1,"language":{"id":2,"name":"French"}}`))

	if !series.HasLanguage() {
		t.Fatal("v5 series language must be kept")
	}
}

func TestFillProfileSizesKeepsUnconfigured(t *testing.T) {
	t.Parallel()

	source := mustUnmarshal[sonarr.QualityProfileResource](t, `{"items":[
	  {"quality":{"id":1},"items":[],"minSize":2,"maxSize":100,"preferredSize":95},
	  {"name":"WEB 1080p","items":[{"quality":{"id":3},"items":[],"minSize":4,"maxSize":85,"preferredSize":60}]},
	  {"quality":{"id":9},"items":[],"minSize":null,"maxSize":null,"preferredSize":null}]}`)
	request := mustUnmarshal[sonarr.QualityProfileResource](t, `{"items":[
	  {"quality":{"id":1},"items":[],"allowed":false},
	  {"name":"WEB 1080p","items":[{"quality":{"id":3},"items":[],"minSize":7}]},
	  {"quality":{"id":9},"items":[]}]}`)
	fillProfileSizes(request, source)

	single, grouped, empty := request.Items[0], request.Items[1].Items[0], request.Items[2]
	if single.GetMinSize() != 2 || single.GetMaxSize() != 100 || single.GetPreferredSize() != 95 {
		t.Fatalf("unconfigured (not allowed) quality: Sonarr's sizes, got %v/%v/%v", single.GetMinSize(), single.GetMaxSize(), single.GetPreferredSize())
	}

	if grouped.GetMinSize() != 7 || grouped.GetMaxSize() != 85 || grouped.GetPreferredSize() != 60 {
		t.Fatalf("configured min stays, the rest from Sonarr, got %v/%v/%v", grouped.GetMinSize(), grouped.GetMaxSize(), grouped.GetPreferredSize())
	}

	if v, ok := empty.GetMinSizeOk(); !ok || v != nil {
		t.Fatal("a null size in Sonarr stays null")
	}

	noSizes := mustUnmarshal[sonarr.QualityProfileResource](t, `{"items":[{"quality":{"id":1},"items":[]}]}`)
	fillProfileSizes(noSizes, mustUnmarshal[sonarr.QualityProfileResource](t, `{"items":[{"quality":{"id":1},"items":[]}]}`))

	if out, _ := json.Marshal(noSizes); strings.Contains(string(out), "Size") {
		t.Fatalf("v4 source: nothing copied: %s", out)
	}
}
