package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	bunny "github.com/Aniem-Couple-of-Coders/Go-Module-Bunny"
	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

const (
	keyUserID             = "user_id"
	keyPassword           = "password"
	keyDeleted            = "deleted"
	keyStorageUsed        = "storage_used"
	keyFilesStored        = "files_stored"
	keyRegion             = "region"
	keyReplicationRegions = "replication_regions"
	keyZoneTier 	      = "zonetier"
	keyReadOnlyPassword   = "read_only_password"
	keyCustom404FilePath  = "custom_404_file_path"
	keyRewrite404To200    = "rewrite_404_to_200"
)

var (
	storageZoneAllRegions = []string{
		"AZ",
		"BR",
		"DE",
		"LA",
		"NY",
		"SE",
		"SG",
		"SYD",
		"UK",
	}
	storageZoneRegionsRequiringReplication = map[string]struct{}{
		"AZ":  {},
		"BR":  {},
		"SG":  {},
		"SYD": {},
	}
)

func resourceStorageZone() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceStorageZoneCreate,
		ReadContext:   resourceStorageZoneRead,
		UpdateContext: resourceStorageZoneUpdate,
		DeleteContext: resourceStorageZoneDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

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
				Type: schema.TypeString,
				Description: fmt.Sprintf(
					"The code of the main storage zone region (Possible values: %s).",
					strings.Join(storageZoneAllRegions, ", "),
				),
				Optional: true,
				Default:  "DE",
				ValidateDiagFunc: validation.ToDiagFunc(
					validation.StringInSlice(storageZoneAllRegions, false),
				),
			},
			keyReplicationRegions: {
				Type: schema.TypeSet,
				Description: fmt.Sprintf(
					"The list of replication zones for the storage zone (Possible values: %s). Replication zones cannot be removed once the zone has been created.",
					strings.Join(storageZoneAllRegions, ", "),
				),
				Elem: &schema.Schema{
					Type: schema.TypeString,
					ValidateDiagFunc: validation.ToDiagFunc(
						validation.StringInSlice(storageZoneAllRegions, false),
					),
				},
				Optional: true,
			},
			keyZoneTier: {
				Type: 		schema.TypeInt,
				Description: 	fmt.Sprintf("The zone tier of the storage, 0 for HDD and 1 for SSD."),
				Required:    	true,
			},

			// mutable properties
			keyOriginURL: {
				Type:        schema.TypeString,
				Description: "A URL to which a request is proxied, if a file does not exist in the the storage zone.",
				Optional:    true,
			},
			keyCustom404FilePath: {
				Type:        schema.TypeString,
				Description: "The path to the custom file that will be returned in a case of 404.",
				Optional:    true,
				Default:     "/bunnycdn_errors/404.html",
			},
			keyRewrite404To200: {
				Type:        schema.TypeBool,
				Description: "Rewrite 404 status code to 200 for URLs without extension.",
				Optional:    true,
				Default:     false,
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
			customdiff.ValidateChange(keyZoneTier, func(_ context.Context, old interface{}, new interface{}, meta interface{}) error {
				return validateImmutableStringProperty(keyZoneTier, old, new)
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
			customdiff.IfValue(
				keyReplicationRegions,
				func(ctx context.Context, value, meta interface{}) bool {
					var regions *schema.Set = value.(*schema.Set)
					return regions.Len() == 0
				},
				customdiff.ValidateValue(keyRegion, func(_ context.Context, value interface{}, meta interface{}) error {
					region := value.(string)
					if _, ok := storageZoneRegionsRequiringReplication[region]; ok {
						return creatingRegionWithoutReplicationRegionError(region, removeValueFromStringSlice(storageZoneAllRegions, region))
					}

					return nil
				}),
			),
			customdiff.If(
				func(ctx context.Context, d *schema.ResourceDiff, meta interface{}) bool {
					region := d.Get(keyRegion).(string)
					regions := d.Get(keyReplicationRegions).(*schema.Set)

					return regions.Contains(region)
				},
				customdiff.ValidateValue(keyRegion, func(_ context.Context, value interface{}, meta interface{}) error {
					return creatingRegionWithReplicationInSameRegionError(value.(string))
				}),
			),
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
	const message = "'%s' is immutable and cannot be changed from '%s' to '%s'.\n" +
		"To change the existing '%s' the 'bunny_storagezone' must be deleted and recreated.\n" +
		"WARNING: deleting a 'bunny_storagezone' will also delete all the data it contains"
	return fmt.Errorf(message, key, old, new, key)
}

func immutableReplicationRegionError(key string, removed []interface{}) error {
	const message = "'%s' can be added but not removed once the zone has been created.\n" +
		"This error occurred when attempting to remove values %+q from '%s'.\n" +
		"To remove an existing '%s' the 'bunny_storagezone' must be deleted and recreated.\n" +
		"WARNING: deleting a 'bunny_storagezone' will also delete all the data it contains"
	return fmt.Errorf(
		message,
		key,
		removed,
		key,
		key,
	)
}

func creatingRegionWithoutReplicationRegionError(region string, availRegions []string) error {
	const message = "%q region needs to have at least one replication region.\n" +
		"Please add one of the available replication regions: %s"
	return fmt.Errorf(message, region, strings.Join(availRegions, ", "))
}

func creatingRegionWithReplicationInSameRegionError(region string) error {
	const message = "%q was specified as primary and replication region. " +
		"The same region can not be both. Please specify different regions"
	return fmt.Errorf(message, region)
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
		ZoneTier: 	    getStrPtr(d, keyZoneTier),
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

	storageZone := storageZoneFromResource(d)

	id, err := getIDAsInt64(d)
	if err != nil {
		return diag.FromErr(err)
	}

	updateErr := clt.StorageZone.Update(ctx, id, storageZone)
	if updateErr != nil {
		// The storagezone contains fields /custom_404_file_path) that are only updated by the
		// provider and not retrieved via ReadContext(). This causes that we run into the bug
		// https://github.com/hashicorp/terraform-plugin-sdk/issues/476.
		// As workaround d.Partial(true) is called.
		d.Partial(true)
		return diagsErrFromErr("updating storage zone via API failed", updateErr)
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
	if err := d.Set(keyZoneTier, sz.ZoneTier); err != nil {
		return err
	}

	return nil
}

// storageZoneFromResource returns a StorageZoneUpdateOptions API type that
// has fields set to the values in d.
func storageZoneFromResource(d *schema.ResourceData) *bunny.StorageZoneUpdateOptions {
	return &bunny.StorageZoneUpdateOptions{
		ReplicationRegions: getStrSetAsSlice(d, keyReplicationRegions),
		OriginURL:          getOkStrPtr(d, keyOriginURL),
		Custom404FilePath:  getOkStrPtr(d, keyCustom404FilePath),
		Rewrite404To200:    getBoolPtr(d, keyRewrite404To200),
	}
}

func removeValueFromStringSlice(values []string, value string) []string {
	result := make([]string, 0, len(values))

	for _, v := range values {
		if v != value {
			result = append(result, v)
		}
	}

	return result
}
