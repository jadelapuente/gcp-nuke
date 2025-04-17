package resources

import (
	"context"
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"

	"google.golang.org/api/iterator"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"

	liberror "github.com/ekristen/libnuke/pkg/errors"
	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"

	"github.com/ekristen/gcp-nuke/pkg/nuke"
)

const VPCRouterResource = "VPCRouter"

func init() {
	registry.Register(&registry.Registration{
		Name:     VPCRouterResource,
		Scope:    nuke.Project,
		Resource: &VPCRouter{},
		Lister:   &VPCRouterLister{},
		DependsOn: []string{
			VPCNATResource,
		},
	})
}

type VPCRouterLister struct {
	svc *compute.RoutersClient
}

func (l *VPCRouterLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	var resources []resource.Resource

	opts := o.(*nuke.ListerOpts)
	if err := opts.BeforeList(nuke.Regional, "compute.googleapis.com"); err != nil {
		return resources, err
	}

	if l.svc == nil {
		var err error
		l.svc, err = compute.NewRoutersRESTClient(ctx, opts.ClientOptions...)
		if err != nil {
			return nil, err
		}
	}

	req := &computepb.ListRoutersRequest{
		Project: *opts.Project,
		Region:  *opts.Region,
	}
	it := l.svc.List(ctx, req)
	for {
		resp, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			logrus.WithError(err).Error("unable to iterate networks")
			break
		}

		resources = append(resources, &VPCRouter{
			svc:     l.svc,
			Project: opts.Project,
			Region:  opts.Region,
			Name:    resp.Name,
		})
	}

	return resources, nil
}

type VPCRouter struct {
	svc      *compute.RoutersClient
	removeOp *compute.Operation
	Project  *string
	Region   *string
	Name     *string
}

func (r *VPCRouter) Remove(ctx context.Context) error {
	var err error
	r.removeOp, err = r.svc.Delete(ctx, &computepb.DeleteRouterRequest{
		Project: *r.Project,
		Region:  *r.Region,
		Router:  *r.Name,
	})
	if err != nil {
		return err
	}

	return r.HandleWait(ctx)
}

func (r *VPCRouter) HandleWait(ctx context.Context) error {
	if r.removeOp == nil {
		return nil
	}

	if err := r.removeOp.Poll(ctx); err != nil {
		logrus.WithError(err).Trace("router remove op polling encountered error")
		return err
	}

	if !r.removeOp.Done() {
		return liberror.ErrWaitResource("waiting for operation to complete")
	}

	if r.removeOp.Done() {
		if r.removeOp.Proto().GetError() != nil {
			removeErr := fmt.Errorf("delete error on '%s': %s", r.removeOp.Proto().GetTargetLink(), r.removeOp.Proto().GetHttpErrorMessage())
			logrus.WithError(removeErr).WithField("status_code", r.removeOp.Proto().GetError()).Error("unable to delete router")
			return removeErr
		}
	}

	return nil
}

func (r *VPCRouter) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *VPCRouter) String() string {
	return *r.Name
}
