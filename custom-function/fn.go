// Package main implements a Crossplane composition function that automatically
// injects standard labels onto all composed resources in a pipeline.
package main

import (
	"context"

	"github.com/crossplane/function-sdk-go/errors"
	"github.com/crossplane/function-sdk-go/logging"
	fnv1beta1 "github.com/crossplane/function-sdk-go/proto/v1beta1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/response"
)

// Function implements the auto-labeler composition function.
type Function struct {
	fnv1beta1.UnimplementedFunctionRunnerServiceServer
	log logging.Logger
}

// Input holds the desired labels to inject, supplied via the composition pipeline step.
type Input struct {
	// Labels is the map of labels to add to every composed resource.
	Labels map[string]string `json:"labels"`
}

// RunFunction is the gRPC handler called by the Crossplane composition engine.
// It reads the desired composed resources from the request, merges the labels
// defined in the function input, and writes them back to the response.
func (f *Function) RunFunction(_ context.Context, req *fnv1beta1.RunFunctionRequest) (*fnv1beta1.RunFunctionResponse, error) {
	log := f.log.WithValues("tag", req.GetMeta().GetTag())
	log.Info("Running auto-labeler function")

	rsp := response.To(req, response.DefaultTTL)

	// Decode the function input to get the desired labels.
	in := &Input{}
	if err := request.GetInput(req, in); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot decode function input"))
		return rsp, nil
	}

	if len(in.Labels) == 0 {
		log.Info("No labels specified in input; nothing to do")
		return rsp, nil
	}

	// Retrieve the map of desired composed resources built by previous pipeline steps.
	desired, err := request.GetDesiredComposedResources(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get desired composed resources"))
		return rsp, nil
	}

	// Merge the requested labels into every desired composed resource.
	for name, dr := range desired {
		if dr.Resource == nil {
			continue
		}

		existing := dr.Resource.GetLabels()
		if existing == nil {
			existing = make(map[string]string)
		}

		for k, v := range in.Labels {
			existing[k] = v
		}

		dr.Resource.SetLabels(existing)
		desired[name] = dr

		log.Info("Injected labels onto composed resource",
			"resource", name,
			"labels", in.Labels,
		)
	}

	// Write the mutated desired resources back to the response.
	if err := response.SetDesiredComposedResources(rsp, desired); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot set desired composed resources"))
		return rsp, nil
	}

	// Propagate the composite resource's own desired state unchanged.
	oxr, err := request.GetObservedCompositeResource(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get observed composite resource"))
		return rsp, nil
	}

	dxr, err := request.GetDesiredCompositeResource(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get desired composite resource"))
		return rsp, nil
	}

	// Carry forward the composite resource's labels as well.
	cxrLabels := dxr.Resource.GetLabels()
	if cxrLabels == nil {
		cxrLabels = make(map[string]string)
	}
	for k, v := range in.Labels {
		cxrLabels[k] = v
	}
	dxr.Resource.SetLabels(cxrLabels)

	// Preserve existing composite status fields.
	_ = oxr

	if err := response.SetDesiredCompositeResource(rsp, &resource.DesiredComposite{Resource: dxr.Resource}); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot set desired composite resource"))
		return rsp, nil
	}

	log.Info("Auto-labeler function completed successfully")
	return rsp, nil
}
