// Ports AUnrealFeatures.Ares/Models/PagedResponse.cs (PageData, PagedResponse<T>)
// and the concrete pages built on it: DeploymentsResponsePage.cs,
// FleetResponsePage.cs, ServerDeploymentResponsePage.cs,
// ServerEventResponsePage.cs, StationResponsePage.cs, UsersResponsePage.cs.
//
// Package models is the Go port of the AUnrealFeatures.Ares.Models namespace.
// Type and field names equal the C# names; JSON names equal the
// [JsonPropertyName] values (or the C# property name when absent).
package models

import "solara/internal/dotnet"

// PageData mirrors AUnrealFeatures.Ares.Models.PageData.
type PageData struct {
	TotalItems int `json:"total_items"`
	ItemCount  int `json:"item_count"`
	PageSize   int `json:"page_size"`
	Page       int `json:"page"`
	Pages      int `json:"pages"`
}

func NewPageData() *PageData { return &PageData{} }

// PagedResponse mirrors the abstract PagedResponse<T>. Concrete pages embed it.
type PagedResponse[T any] struct {
	Page  *PageData `json:"page"`
	Items []T       `json:"items"`
}

// Paged gives Create access to the embedded PagedResponse of a concrete page.
func (p *PagedResponse[T]) Paged() *PagedResponse[T] { return p }

// PagedPtr is satisfied by *R where R embeds PagedResponse[T]
// (C#: `where TResponse : PagedResponse<T>`).
type PagedPtr[T any, R any] interface {
	*R
	Paged() *PagedResponse[T]
}

// Create mirrors PagedResponse<T>.Create<TResponse>(items, pageSize, defaultPage = 1).
// Callers pass defaultPage explicitly (the C# default is 1). The thrown
// exceptions match the original (they surface as the 500 page).
func Create[T any, R any, PR PagedPtr[T, R]](items []T, pageSize int, defaultPage int) *R {
	if items == nil {
		dotnet.Throw("System.ArgumentNullException", "Value cannot be null. (Parameter 'items')")
	}
	if pageSize <= 0 {
		dotnet.Throw("System.ArgumentException", "Page size must be greater than 0 (Parameter 'pageSize')")
	}
	if defaultPage <= 0 {
		dotnet.Throw("System.ArgumentException", "Page number must be greater than 0 (Parameter 'defaultPage')")
	}
	totalItems := len(items)
	// (int)Math.Ceiling(totalItems / (double)pageSize)
	totalPages := totalItems / pageSize
	if totalItems%pageSize != 0 {
		totalPages++
	}
	defaultPage = min(defaultPage, max(totalPages, 1))
	// (defaultPage - 1) * pageSize is int arithmetic in C# (wraps on overflow);
	// Skip with a negative count skips nothing.
	skip := int(int32((defaultPage - 1) * pageSize))
	if skip < 0 {
		skip = 0
	}
	paged := []T{}
	if skip < totalItems {
		end := totalItems
		if pageSize < totalItems-skip {
			end = skip + pageSize
		}
		paged = append(paged, items[skip:end]...)
	}
	activator := PR(new(R))
	p := activator.Paged()
	p.Items = paged
	p.Page = &PageData{
		TotalItems: totalItems,
		ItemCount:  len(paged),
		PageSize:   pageSize,
		Page:       defaultPage,
		Pages:      totalPages,
	}
	return (*R)(activator)
}

// DeploymentsResponsePage mirrors DeploymentsResponsePage : PagedResponse<DeploymentExpanded>.
type DeploymentsResponsePage struct {
	PagedResponse[*DeploymentExpanded]
	StationId *string `json:"station_id"`
}

func NewDeploymentsResponsePage() *DeploymentsResponsePage { return &DeploymentsResponsePage{} }

// FleetResponsePage mirrors FleetResponsePage : PagedResponse<FleetResponse>.
type FleetResponsePage struct {
	PagedResponse[*FleetResponse]
}

func NewFleetResponsePage() *FleetResponsePage { return &FleetResponsePage{} }

// ServerDeploymentResponsePage mirrors ServerDeploymentResponsePage : PagedResponse<ServerDeploymentResponse>.
type ServerDeploymentResponsePage struct {
	PagedResponse[*ServerDeploymentResponse]
}

func NewServerDeploymentResponsePage() *ServerDeploymentResponsePage {
	return &ServerDeploymentResponsePage{}
}

// ServerEventResponsePage mirrors ServerEventResponsePage : PagedResponse<ServerEventResponse>.
type ServerEventResponsePage struct {
	PagedResponse[*ServerEventResponse]
}

func NewServerEventResponsePage() *ServerEventResponsePage { return &ServerEventResponsePage{} }

// StationResponsePage mirrors StationResponsePage : PagedResponse<StationResponse>.
type StationResponsePage struct {
	PagedResponse[*StationResponse]
}

func NewStationResponsePage() *StationResponsePage { return &StationResponsePage{} }

// UsersResponsePage mirrors UsersResponsePage : PagedResponse<UserDataResponse>.
type UsersResponsePage struct {
	PagedResponse[*UserDataResponse]
}

func NewUsersResponsePage() *UsersResponsePage { return &UsersResponsePage{} }
