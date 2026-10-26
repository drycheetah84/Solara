// Ports Models/MothershipOfferDbObject.cs, MothershipOfferBindingDbObject.cs,
// MothershipOfferDisplayDbObject.cs, MothershipEntitlementDbObject.cs,
// MothershipTransactionCatalogDbObject.cs and MothershipInventoryItemDbObject.cs.
package models

import "solara/internal/docdb"

// MothershipOfferDbObject is a store offer.
type MothershipOfferDbObject struct {
	docdb.AstraDbObject
	OfferId           *string `json:"offer_id"`
	Name              *string `json:"name"`
	TitleId           *string `json:"titleId"`
	EnvId             *string `json:"envId"`
	TransactionId     *string `json:"transaction_id"`
	BundlePricingJson *string `json:"bundle_pricing"`
	DiscountPercent   int     `json:"discount_percent"`
}

func NewMothershipOfferDbObject() *MothershipOfferDbObject {
	o := &MothershipOfferDbObject{BundlePricingJson: P("{}")}
	initStrings(o)
	return o
}

// MothershipOfferBindingDbObject binds an offer to a deployment.
type MothershipOfferBindingDbObject struct {
	docdb.AstraDbObject
	OfferBindingId *string `json:"offer_binding_id"`
	TitleId        *string `json:"title_id"`
	EnvId          *string `json:"env_id"`
	DeploymentId   *string `json:"deployment_id"`
	OfferDisplayId *string `json:"offer_display_id"`
	OfferId        *string `json:"offer_id"`
	Committed      bool    `json:"committed"`
	DisplayIndex   int     `json:"display_index"`
}

func NewMothershipOfferBindingDbObject() *MothershipOfferBindingDbObject {
	o := &MothershipOfferBindingDbObject{}
	initStrings(o)
	return o
}

// MothershipOfferDisplayDbObject is an offer display entry.
type MothershipOfferDisplayDbObject struct {
	docdb.AstraDbObject
	OfferDisplayId *string `json:"offer_display_id"`
	Name           *string `json:"name"`
	Description    *string `json:"description"`
}

func NewMothershipOfferDisplayDbObject() *MothershipOfferDisplayDbObject {
	o := &MothershipOfferDisplayDbObject{}
	initStrings(o)
	return o
}

// MothershipEntitlementDbObject is an entitlement catalog entry.
type MothershipEntitlementDbObject struct {
	docdb.AstraDbObject
	EntitlementId *string `json:"entitlement_id"`
	Name          *string `json:"name"`
	Type          *string `json:"type"`
	ItemClass     *string `json:"item_class"`
}

func NewMothershipEntitlementDbObject() *MothershipEntitlementDbObject {
	o := &MothershipEntitlementDbObject{Type: P("DURABLE")}
	initStrings(o)
	return o
}

// MothershipTransactionCatalogDbObject is a transaction catalog entry.
type MothershipTransactionCatalogDbObject struct {
	docdb.AstraDbObject
	TransactionId  *string `json:"transaction_id"`
	Name           *string `json:"name"`
	OperationsJson *string `json:"operations"`
}

func NewMothershipTransactionCatalogDbObject() *MothershipTransactionCatalogDbObject {
	o := &MothershipTransactionCatalogDbObject{OperationsJson: P("[]")}
	initStrings(o)
	return o
}

// MothershipInventoryItemDbObject is one inventory item of a player.
type MothershipInventoryItemDbObject struct {
	docdb.AstraDbObject
	PlayerId     *string `json:"player_id"`
	ItemId       *string `json:"item_id"`
	ItemDataJson *string `json:"item_data"`
}

func NewMothershipInventoryItemDbObject() *MothershipInventoryItemDbObject {
	o := &MothershipInventoryItemDbObject{ItemDataJson: P("{}")}
	initStrings(o)
	return o
}
