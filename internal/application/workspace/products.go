package workspace

import (
	"context"

	domain "github.com/denislibs/papirus-identity-center/internal/domain/workspace"
)

type ListProducts struct{ products domain.ProductRepository }

func NewListProducts(p domain.ProductRepository) *ListProducts { return &ListProducts{p} }

func (uc *ListProducts) Execute(ctx context.Context) ([]*domain.Product, error) {
	return uc.products.ListAll(ctx)
}

type EnableProduct struct {
	members  domain.MemberRepository
	products domain.ProductRepository
	wp       domain.WorkspaceProductRepository
}

func NewEnableProduct(m domain.MemberRepository, p domain.ProductRepository, wp domain.WorkspaceProductRepository) *EnableProduct {
	return &EnableProduct{members: m, products: p, wp: wp}
}

func (uc *EnableProduct) Execute(ctx context.Context, wsID, requesterID, productKey string) error {
	if err := requireManager(ctx, uc.members, wsID, requesterID); err != nil {
		return err
	}
	ok, err := uc.products.Exists(ctx, productKey)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrProductNotFound
	}
	return uc.wp.Enable(ctx, wsID, productKey)
}

type DisableProduct struct {
	members domain.MemberRepository
	wp      domain.WorkspaceProductRepository
}

func NewDisableProduct(m domain.MemberRepository, wp domain.WorkspaceProductRepository) *DisableProduct {
	return &DisableProduct{members: m, wp: wp}
}

func (uc *DisableProduct) Execute(ctx context.Context, wsID, requesterID, productKey string) error {
	if err := requireManager(ctx, uc.members, wsID, requesterID); err != nil {
		return err
	}
	return uc.wp.Disable(ctx, wsID, productKey)
}

type ListEnabledProducts struct {
	members domain.MemberRepository
	wp      domain.WorkspaceProductRepository
}

func NewListEnabledProducts(m domain.MemberRepository, wp domain.WorkspaceProductRepository) *ListEnabledProducts {
	return &ListEnabledProducts{members: m, wp: wp}
}

func (uc *ListEnabledProducts) Execute(ctx context.Context, wsID, requesterID string) ([]*domain.Product, error) {
	if err := requireMember(ctx, uc.members, wsID, requesterID); err != nil {
		return nil, err
	}
	return uc.wp.ListEnabled(ctx, wsID)
}
