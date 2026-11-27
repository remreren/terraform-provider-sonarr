package provider

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// sonarrMajor is the major version of the Sonarr under test (SONARR_URL).
func sonarrMajor(t *testing.T) int {
	t.Helper()

	status, _, err := testAccAPIClient().SystemAPI.GetSystemStatus(context.Background()).Execute()
	if err != nil {
		t.Fatalf("system status: %v", err)
	}

	var major int

	fmt.Sscanf(status.GetVersion(), "%d", &major)

	return major
}

// Media management with user_rejected_extensions: on v5 it is applied and read back; on v4 the
// field doesn't exist, the plan is kept. Before the fix v5 answered 500 on every create.
func TestAccMediaManagementUserRejectedExtensions(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccMediaManagementResourceConfig("none"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("sonarr_media_management.test", "user_rejected_extensions", ""),
				),
			},
			{
				Config: strings.Replace(testAccMediaManagementResourceConfig("none"), "file_date", `user_rejected_extensions = "exe,lnk"
		file_date`, 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("sonarr_media_management.test", "user_rejected_extensions", "exe,lnk"),
					checkUserRejectedExtensionsInSonarr(t, "exe,lnk"),
				),
			},
		},
	})
}

func checkUserRejectedExtensionsInSonarr(t *testing.T, want string) resource.TestCheckFunc {
	t.Helper()

	return func(_ *terraform.State) error {
		if sonarrMajor(t) < 5 {
			return nil
		}

		cfg, _, err := testAccAPIClient().MediaManagementConfigAPI.GetMediaManagementConfig(context.Background()).Execute()
		if err != nil {
			return err
		}

		if cfg.GetUserRejectedExtensions() != want {
			return fmt.Errorf("Sonarr has userRejectedExtensions %q, want %q", cfg.GetUserRejectedExtensions(), want)
		}

		return nil
	}
}

// Quality profile sizes per quality (Sonarr v5): set on a single quality and on a quality inside a
// group, read back from Sonarr, no diff on the next plan.
func TestAccQualityProfileSizesV5(t *testing.T) {
	testAccPreCheck(t)

	if sonarrMajor(t) < 5 {
		t.Skip("per-profile sizes need Sonarr v5+")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccQualityProfileSizesConfig(4, 85, 60),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("sonarr_quality_profile.sizes", "quality_groups.0.qualities.0.min_size", "4"),
					resource.TestCheckResourceAttr("sonarr_quality_profile.sizes", "quality_groups.0.qualities.0.max_size", "85"),
					resource.TestCheckResourceAttr("sonarr_quality_profile.sizes", "quality_groups.0.qualities.0.preferred_size", "60"),
					resource.TestCheckResourceAttr("sonarr_quality_profile.sizes", "quality_groups.1.qualities.0.min_size", "3"),
					resource.TestCheckResourceAttr("sonarr_quality_profile.sizes", "quality_groups.1.qualities.0.max_size", "90"),
					checkProfileSizesInSonarr(t, "SizesTest", "WEBDL-1080p", 4, 85, 60),
					checkProfileSizesInSonarr(t, "SizesTest", "Bluray-1080p", 3, 90, 70),
				),
			},
			// Update
			{
				Config: testAccQualityProfileSizesConfig(5, 80, 50),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("sonarr_quality_profile.sizes", "quality_groups.0.qualities.0.min_size", "5"),
					checkProfileSizesInSonarr(t, "SizesTest", "WEBDL-1080p", 5, 80, 50),
				),
			},
			// ImportState
			{
				ResourceName:      "sonarr_quality_profile.sizes",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func checkProfileSizesInSonarr(t *testing.T, profile, quality string, minSize, maxSize, preferred float64) resource.TestCheckFunc {
	t.Helper()

	return func(_ *terraform.State) error {
		profiles, _, err := testAccAPIClient().QualityProfileAPI.ListQualityProfile(context.Background()).Execute()
		if err != nil {
			return err
		}

		for _, p := range profiles {
			if p.GetName() != profile {
				continue
			}

			for _, item := range p.GetItems() {
				all := append(item.GetItems(), item)
				for _, q := range all {
					if q.Quality == nil || q.Quality.GetName() != quality {
						continue
					}

					if q.GetMinSize() != minSize || q.GetMaxSize() != maxSize || q.GetPreferredSize() != preferred {
						return fmt.Errorf("%s in %s: Sonarr has %v/%v/%v, want %v/%v/%v", quality, profile,
							q.GetMinSize(), q.GetMaxSize(), q.GetPreferredSize(), minSize, maxSize, preferred)
					}

					return nil
				}
			}

			return fmt.Errorf("%s not in profile %s", quality, profile)
		}

		return fmt.Errorf("profile %s not found", profile)
	}
}

func testAccQualityProfileSizesConfig(minSize, maxSize, preferred int) string {
	return fmt.Sprintf(`
	data "sonarr_quality" "webdl" {
		name = "WEBDL-1080p"
	}

	data "sonarr_quality" "webrip" {
		name = "WEBRip-1080p"
	}

	data "sonarr_quality" "bluray" {
		name = "Bluray-1080p"
	}

	resource "sonarr_quality_profile" "sizes" {
		name            = "SizesTest"
		upgrade_allowed = true
		cutoff          = 2001

		quality_groups = [
			{
				id   = 2001
				name = "WEB 1080p"
				qualities = [
					merge(data.sonarr_quality.webdl, { min_size = %d, max_size = %d, preferred_size = %d }),
					data.sonarr_quality.webrip,
				]
			},
			{
				qualities = [
					merge(data.sonarr_quality.bluray, { min_size = 3, max_size = 90, preferred_size = 70 }),
				]
			},
		]
	}
	`, minSize, maxSize, preferred)
}
