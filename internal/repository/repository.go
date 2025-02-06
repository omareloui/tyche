package repository

import (
	tenantmanager "github.com/omareloui/tyche/internal/tenant_manager"
)

type Repository struct {
	tm *tenantmanager.TenantManager
}

func New(tm *tenantmanager.TenantManager) *Repository {
	return &Repository{tm: tm}
}
