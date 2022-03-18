package helm

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	helmcmd "github.com/helm/helm-2to3/cmd"
	v2 "github.com/helm/helm-2to3/pkg/v2"
	"github.com/pkg/errors"
)

func resourceMigration() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceMigrationCreate,
		ReadContext:   resourceMigrationRead,
		UpdateContext: resourceMigrationUpdate,
		DeleteContext: resourceMigrationDelete,
		//CustomizeDiff:        //TODO,
		Schema: map[string]*schema.Schema{
			"name": {
				Type:        schema.TypeString,
				ForceNew:    true,
				Required:    true,
				Description: "Release name.",
			},
			"namespace": {
				Type:        schema.TypeString,
				ForceNew:    true,
				Required:    true,
				Description: "Helm3 release namespace.",
			},
			"tiller_namespace": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Default:     "kube-system",
				Description: "The tiller namespace. Defaults to kube-system",
			},
			"delete_v2_releases": {
				Type:        schema.TypeBool,
				ForceNew:    true,
				Optional:    true,
				Default:     false,
				Description: "v2 release versions are deleted after migration. By default, the v2 release versions are retained",
			},
			"ignore_already_migrated": {
				Type:        schema.TypeBool,
				ForceNew:    false,
				Optional:    true,
				Default:     false,
				Description: "Ignore any already migrated release versions and continue migrating",
			},
			"max_release_versions": {
				Type:        schema.TypeInt,
				ForceNew:    false,
				Optional:    true,
				Default:     10,
				Description: "Ignore any already migrated release versions and continue migrating",
			},
			"v2exists": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Track if the v2 release exists",
			},
			"v3exists": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Track if the v3 release exists",
			},
			"migration_complete": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Track if migration is complete",
			},
			"cleanup_complete": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Track if the v2 release has been cleaned up",
			},
		},
	}
}

func resourceMigrationRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	//log.Printf("Release \"%s\" will be converted from Helm v2 to Helm v3.\n", convertOptions.ReleaseName)
	//log.Printf("[Helm 3] Release \"%s\" will be created.\n", convertOptions.ReleaseName)

	logID := fmt.Sprintf("[resourceMigrationRead: %s]", d.Get("name").(string))
	debug("%s Started", logID)

	m := meta.(*Meta)
	n := d.Get("namespace").(string)

	name := d.Get("name").(string)
	// If the v2 release doesn't exist, error.
	retrieveOptions := v2.RetrieveOptions{
		ReleaseName:     name,
		TillerNamespace: d.Get("tiller_namespace").(string),
	}
	// We can get a helm.KubeConfig type from....
	/// 	kc, err := newKubeConfig(m.data, &namespace)
	kubeConfig, err := m.GetHelmV2ConfigurationInfo()
	if err != nil {
		return diag.FromErr(err)
	}

	helm3Config, err := m.GetHelmConfiguration(n)
	if err != nil {
		return diag.FromErr(err)
	}

	v2Found := false
	v3Found := false
	_, v2FindErr := v2.GetReleaseVersions(retrieveOptions, *kubeConfig)
	_, v3FindErr := getRelease(m, helm3Config, name)
	if v2FindErr == nil {
		v2Found = true
		if err := d.Set("v2exists", true); err != nil {
			return diag.FromErr(err)
		}
	}
	if v3FindErr == nil {
		v3Found = true
		if err := d.Set("v3exists", true); err != nil {
			return diag.FromErr(err)
		}
	}
	if !v2Found && !v3Found {
		debug("Could not find v2 or v3 release")
		return diag.FromErr(errors.New("couldn't find either release."))
	}
	id := fmt.Sprintf("%s/%s", n, name)
	if v2Found && !v3Found {
		if err := d.Set("v2exists", true); err != nil {
			return diag.FromErr(err)
		}
	}
	if !v2Found && v3Found {
		if err := d.Set("v2exists", false); err != nil {
			return diag.FromErr(err)
		}
		if err := d.Set("v3exists", true); err != nil {
			return diag.FromErr(err)
		}
	}
	d.SetId(id)
	return diag.Diagnostics{}
}

func resourceMigrationCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	logID := fmt.Sprintf("[resourceMigrationCreate: %s]", d.Get("name").(string))
	debug("%s Started", logID)

	m := meta.(*Meta)

	name := d.Get("name").(string)
	tillerNS := d.Get("tiller_namespace").(string)
	ignoreAlreadyMigrated := d.Get("ignore_already_migrated").(bool)
	maxReleaseVersions := d.Get("max_release_versions").(int)
	deleteV2Releases := d.Get("delete_v2_releases").(bool)

	kubeConfig, err := m.GetHelmV2ConfigurationInfo()
	if err != nil {
		return diag.FromErr(err)
	}
	convertOptions := helmcmd.ConvertOptions{
		DeleteRelease:         deleteV2Releases,
		MaxReleaseVersions:    maxReleaseVersions,
		ReleaseName:           name,
		TillerNamespace:       tillerNS,
		IgnoreAlreadyMigrated: ignoreAlreadyMigrated,
	}
	if err := helmcmd.Convert(convertOptions, *kubeConfig); err != nil {
		debug("Error with helm convert.")
		return diag.FromErr(err)
	}
	if err := d.Set("migration_complete", true); err != nil {
		diag.FromErr(err)
	}
	if deleteV2Releases {
		if err := d.Set("cleanup_complete", true); err != nil {
			diag.FromErr(err)
		}
	}
	return resourceMigrationRead(ctx, d, meta)
}

// The only update we should handle is
func resourceMigrationUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return nil
}

// Deleting this means nothing.
func resourceMigrationDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return nil
}

func resourceMigrationExists(d *schema.ResourceData, meta interface{}) (bool, error) {
	logID := fmt.Sprintf("[resourceMigraitonExists: %s]", d.Get("name").(string))
	debug("%s Start", logID)

	m := meta.(*Meta)
	n := d.Get("namespace").(string)

	c, err := m.GetHelmConfiguration(n)
	if err != nil {
		return false, err
	}

	name := d.Get("name").(string)
	_, err = getRelease(m, c, name)

	debug("%s Done", logID)

	if err == nil {
		return true, nil
	}

	if err == errReleaseNotFound {
		return false, nil
	}

	return false, err
}
