package provider

import (
	"context"

	"github.com/devopsarr/sonarr-go/sonarr"
	"github.com/devopsarr/terraform-provider-sonarr/internal/helpers"
	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// Sonarr v5 resets a field a PUT leaves out. These resources don't manage the v5+ fields below,
// so before an update they are copied from what Sonarr has now (v4 has none: nothing is copied).

// keepImportListUnset keeps an import list's language and tagExisting (Sonarr v5+).
func keepImportListUnset(ctx context.Context, client *sonarr.APIClient, request *sonarr.ImportListResource, diags *diag.Diagnostics) bool {
	current, _, err := client.ImportListAPI.GetImportListById(ctx, request.GetId()).Execute()
	if err != nil {
		diags.AddError(helpers.ClientError, helpers.ParseClientError(helpers.Read, importListResourceName, err))

		return false
	}

	fillImportListFromCurrent(request, current)

	return true
}

func fillImportListFromCurrent(request, current *sonarr.ImportListResource) {
	if v, ok := current.GetLanguageOk(); ok && !request.HasLanguage() {
		request.SetLanguage(*v)
	}

	if v, ok := current.GetTagExistingOk(); ok && !request.HasTagExisting() {
		request.SetTagExisting(*v)
	}
}

// keepSeriesUnset keeps a series' language (Sonarr v5+).
func keepSeriesUnset(ctx context.Context, client *sonarr.APIClient, request *sonarr.SeriesResource, diags *diag.Diagnostics) bool {
	current, _, err := client.SeriesAPI.GetSeriesById(ctx, request.GetId()).Execute()
	if err != nil {
		diags.AddError(helpers.ClientError, helpers.ParseClientError(helpers.Read, seriesResourceName, err))

		return false
	}

	fillSeriesFromCurrent(request, current)

	return true
}

func fillSeriesFromCurrent(request, current *sonarr.SeriesResource) {
	if v, ok := current.GetLanguageOk(); ok && !request.HasLanguage() {
		request.SetLanguage(*v)
	}
}
