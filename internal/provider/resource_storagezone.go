package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	bunny "github.com/simplesurance/bunny-go"
)

const (
	keyUserID             = "user_id"
	keyPassword           = "password"
	keyDateModified       = "date_modified"
	keyDeleted            = "deleted"
	keyStorageUsed        = "storage_used"
	keyFilesStored        = "files_stored"
	keyRegion             = "region"
	keyReplicationRegions = "replication_regions"
	keyReadOnlyPassword   = "read_only_password"
	keyCustom404FilePath  = "custom_404_file_path"
	keyRewrite404To200    = "rewrite_404_to_200"
)

func resourceStorageZone() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceStorageZoneCreate,
		ReadContext:   resourceStorageZoneRead,
		UpdateContext: resourceStorageZoneUpdate,
		DeleteContext: resourceStorageZoneDelete,

		Schema: map[string]*schema.Schema{
			// immutable properties
			// NOTE: immutable properties are made immutable via
			// validation in the `CustomizeDiff` function. There
			// should be a validation function for each immutable
			// property.
			keyName: {
				Type:        schema.TypeString,
				Description: "The name of the storage zone.",
				Required:    true,
			},
			keyRegion: {
				Type:        schema.TypeString,
				Description: "The code of the main storage zone region (Possible values: DE, NY, LA, SG).",
				Optional:    true,
				Default:     "DE",
				ValidateDiagFunc: validation.ToDiagFunc(
					validation.StringInSlice([]string{"DE", "NY", "LA", "SG"}, false),
				),
			},
			keyReplicationRegions: {
				Type:        schema.TypeSet,
				Description: "The list of replication zones for the storage zone (Possible values: DE, NY, LA, SG, SYD). Replication zones cannot be removed once the zone has been created.",
				Elem: &schema.Schema{
					Type: schema.TypeString,
					ValidateDiagFunc: validation.ToDiagFunc(
						validation.StringInSlice([]string{"DE", "NY", "LA", "SG", "SYD"}, false),
					),
				},
				Optional: true,
			},

			// mutable properties
			keyOriginURL: {
				Type:        schema.TypeString,
				Description: "The origin URL of the storage zone.",
				Optional:    true,
			},
			keyCustom404FilePath: {
				Type:        schema.TypeString,
				Description: "The path to the custom file that will be returned in a case of 404.",
				Optional:    true,
			},
			keyRewrite404To200: {
				Type:        schema.TypeBool,
				Description: "Rewrite 404 status code to 200 for URLs without extension.",
				Optional:    true,
			},

			// computed properties
			keyUserID: {
				Type:     schema.TypeString,
				Computed: true,
			},
			keyPassword: {
				Type:        schema.TypeString,
				Description: "The password granting read/write access to the storage zone.",
				Computed:    true,
				Sensitive:   true,
			},
			keyDateModified: {
				Type:        schema.TypeString,
				Description: "The last modified date of the storage zone.",
				Computed:    true,
			},
			keyDeleted: {
				Type:     schema.TypeBool,
				Computed: true,
			},
			keyStorageUsed: {
				Type:        schema.TypeInt,
				Description: "The amount of storage used in the storage zone in bytes.",
				Computed:    true,
			},
			keyFilesStored: {
				Type:        schema.TypeInt,
				Description: "The number of files stored in the storage zone.",
				Computed:    true,
			},
			keyReadOnlyPassword: {
				Type:        schema.TypeString,
				Description: "The password granting read-only access to the storage zone.",
				Computed:    true,
				Sensitive:   true,
			},
		},

		CustomizeDiff: customdiff.All(
			customdiff.ValidateChange(keyName, func(_ context.Context, old interface{}, new interface{}, meta interface{}) error {
				return validateImmutableStringProperty(keyName, old, new)
			}),
			customdiff.ValidateChange(keyRegion, func(_ context.Context, old interface{}, new interface{}, meta interface{}) error {
				return validateImmutableStringProperty(keyRegion, old, new)
			}),
			customdiff.ValidateChange(keyReplicationRegions, func(_ context.Context, old interface{}, new interface{}, meta interface{}) error {
				if old == nil {
					return nil
				}

				var oldRep *schema.Set = old.(*schema.Set)

				if new == nil {
					return immutableReplicationRegionError(
						keyReplicationRegions,
						oldRep.List(),
					)
				}

				// verify that none of the previous replication regions have been removed.
				var newRep *schema.Set = new.(*schema.Set)
				var intersect *schema.Set = newRep.Intersection(oldRep)
				var removed *schema.Set = oldRep.Difference(intersect)

				// NOTE: oldRep.Equal(intersect) doesn't work for some reason
				areEqual := cmp.Equal(oldRep.List(), intersect.List())
				if !areEqual {
					return immutableReplicationRegionError(
						keyReplicationRegions,
						removed.List(),
					)
				}

				return nil
			}),
		),
	}
}

func validateImmutableStringProperty(key string, old interface{}, new interface{}) error {
	o := old.(string)
	n, nok := new.(string)

	if o == "" {
		return nil
	}

	if new == nil || !nok {
		return immutableStringPropertyError(key, o, "")
	}

	if o != n {
		return immutableStringPropertyError(key, o, n)
	}

	return nil
}

func immutableStringPropertyError(key string, old string, new string) error {
	message := "'%s' is immutable and cannot be changed from '%s' to '%s'. " +
		"If you must change the '%s' of our region you must first delete your resource and then redefine it. " +
		"WARNING: deleting a 'bunny_storagezone' will also delete all the data it contains!"
	return fmt.Errorf(message, key, old, new, key)
}

func immutableReplicationRegionError(key string, removed []interface{}) error {
	message := "'%s' can be added to but not be removed once the zone has been created. " +
		"This error occurred when attempting to remove values %+q from '%s'. " +
		"To remove an existing '%s' the 'bunny_storagezone' must be deleted and recreated. " +
		"WARNING: deleting a 'bunny_storagezone' will also delete all the data it contains!"
	return fmt.Errorf(
		message,
		key,
		removed,
		key,
		key,
	)
}

func resourceStorageZoneCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	clt := meta.(*bunny.Client)

	originURL := getStrPtr(d, keyOriginURL)
	if !d.HasChange(keyOriginURL) {
		originURL = nil
	}

	sz, err := clt.StorageZone.Add(ctx, &bunny.StorageZoneAddOptions{
		Name:               getStrPtr(d, keyName),
		OriginURL:          originURL,
		Region:             getStrPtr(d, keyRegion),
		ReplicationRegions: getStrSetAsSlice(d, keyReplicationRegions),
	})
	if err != nil {
		return diag.FromErr(fmt.Errorf("creating storage zone failed: %w", err))
	}

	d.SetId(strconv.FormatInt(*sz.ID, 10))

	// StorageZone.Add() only supports to set a subset of a Storage Zone object,
	// call Update to set the remaining ones.
	if diags := resourceStorageZoneUpdate(ctx, d, meta); diags.HasError() {
		// if updating fails the sz was still created, initialize with the SZ
		// returned from the Add operation
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "setting storage zone attributes via update failed",
		})

		if err := storageZoneToResource(sz, d); err != nil {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  "converting api-type to resource data failed: " + err.Error(),
			})
		}

		return diags
	}

	return nil
}

func resourceStorageZoneUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	clt := meta.(*bunny.Client)

	storageZone, err := storageZoneFromResource(d)
	if err != nil {
		return diagsErrFromErr("converting resource to API type failed", err)
	}

	id, err := getIDAsInt64(d)
	if err != nil {
		return diag.FromErr(err)
	}

	updateErr := clt.StorageZone.Update(ctx, id, storageZone)
	if updateErr != nil {
		// if our update failed then revert our values to their original
		// state so that we can run an apply again.
		revertErr := revertUpdateValues(d)

		if revertErr != nil {
			return diagsErrFromErr("updating storage zone via API failed", revertErr)
		}

		return diagsErrFromErr("updating storage zone via API failed", updateErr)
	}

	updatedStorageZone, err := clt.StorageZone.Get(ctx, id)
	if err != nil {
		return diagsErrFromErr("fetching updated storage zone via API failed", err)
	}

	if err := storageZoneToResource(updatedStorageZone, d); err != nil {
		return diagsErrFromErr("converting api type to resource data after successful update failed", err)
	}

	return nil
}

func resourceStorageZoneRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	clt := meta.(*bunny.Client)

	id, err := getIDAsInt64(d)
	if err != nil {
		return diag.FromErr(err)
	}

	sz, err := clt.StorageZone.Get(ctx, id)
	if err != nil {
		return diagsErrFromErr("could not retrieve storage zone", err)
	}

	if err := storageZoneToResource(sz, d); err != nil {
		return diagsErrFromErr("converting api type to resource data after successful read failed", err)
	}

	return nil
}

func resourceStorageZoneDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	clt := meta.(*bunny.Client)

	id, err := getIDAsInt64(d)
	if err != nil {
		return diag.FromErr(err)
	}

	err = clt.StorageZone.Delete(ctx, id)
	if err != nil {
		return diagsErrFromErr("could not delete storage zone", err)
	}

	d.SetId("")

	return nil
}

// storageZoneToResource sets fields in d to the values in sz.
func storageZoneToResource(sz *bunny.StorageZone, d *schema.ResourceData) error {
	if sz.ID != nil {
		d.SetId(strconv.FormatInt(*sz.ID, 10))
	}

	if err := d.Set(keyUserID, sz.UserID); err != nil {
		return err
	}
	if err := d.Set(keyName, sz.Name); err != nil {
		return err
	}
	if err := d.Set(keyPassword, sz.Password); err != nil {
		return err
	}
	if err := d.Set(keyDateModified, sz.DateModified); err != nil {
		return err
	}
	if err := d.Set(keyDeleted, sz.Deleted); err != nil {
		return err
	}
	if err := d.Set(keyStorageUsed, sz.StorageUsed); err != nil {
		return err
	}
	if err := d.Set(keyFilesStored, sz.FilesStored); err != nil {
		return err
	}
	if err := d.Set(keyRegion, sz.Region); err != nil {
		return err
	}
	if err := d.Set(keyReadOnlyPassword, sz.ReadOnlyPassword); err != nil {
		return err
	}
	if err := setStrSet(d, keyReplicationRegions, sz.ReplicationRegions, ignoreOrderOpt, caseInsensitiveOpt); err != nil {
		return err
	}

	return nil
}

func revertUpdateValues(d *schema.ResourceData) error {
	o, _ := d.GetChange(keyOriginURL)
	if err := d.Set(keyOriginURL, o); err != nil {
		return err
	}
	o, _ = d.GetChange(keyCustom404FilePath)
	if err := d.Set(keyCustom404FilePath, o); err != nil {
		return err
	}
	o, _ = d.GetChange(keyRewrite404To200)
	if err := d.Set(keyRewrite404To200, o); err != nil {
		return err
	}

	return nil
}

// storageZoneFromResource returns a StorageZoneUpdateOptions API type that
// has fields set to the values in d.
func storageZoneFromResource(d *schema.ResourceData) (*bunny.StorageZoneUpdateOptions, error) {
	var res bunny.StorageZoneUpdateOptions

	res.ReplicationRegions = getStrSetAsSlice(d, keyReplicationRegions)

	if d.HasChange(keyOriginURL) {
		res.OriginURL = getStrPtr(d, keyOriginURL)
	}

	if d.HasChange(keyCustom404FilePath) {
		res.Custom404FilePath = getStrPtr(d, keyCustom404FilePath)
	}

	if d.HasChange(keyRewrite404To200) {
		res.Rewrite404To200 = getBoolPtr(d, keyRewrite404To200)
	}

	return &res, nil
}
