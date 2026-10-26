package main

import (
	"context"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/crossplane/function-sdk-go/errors"
	"github.com/crossplane/function-sdk-go/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/response"

	"github.com/docaohieu2808/crossplane-infra/custom-function/input/v1beta1"
)

// Function stamps the labels from its input on every desired composed resource.
type Function struct {
	fnv1.UnimplementedFunctionRunnerServiceServer

	log logging.Logger
}

// RunFunction runs the Function. It must run after the steps that build the composed resources: it only sees the
// desired state that earlier steps put in the request.
func (f *Function) RunFunction(_ context.Context, req *fnv1.RunFunctionRequest) (*fnv1.RunFunctionResponse, error) {
	f.log.Debug("Running function", "tag", req.GetMeta().GetTag())

	// response.To copies the desired state (and context) of the request, so everything an earlier step set is
	// passed on untouched except what this function edits below.
	rsp := response.To(req, response.DefaultTTL)

	in := &v1beta1.Input{}
	if err := request.GetInput(req, in); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get Function input from %T", req))
		return rsp, nil
	}

	if err := validateLabels(in.Labels); err != nil {
		response.Fatal(rsp, err)
		return rsp, nil
	}

	// Edit the resources in place rather than round-tripping them through request.GetDesiredComposedResources and
	// response.SetDesiredComposedResources: that pair would drop the connection details an earlier step set.
	names := make([]string, 0, len(rsp.GetDesired().GetResources()))
	for name := range rsp.GetDesired().GetResources() {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		r := rsp.GetDesired().GetResources()[name]

		obj := &unstructured.Unstructured{}
		if err := resource.AsObject(r.GetResource(), obj); err != nil {
			response.Fatal(rsp, errors.Wrapf(err, "cannot read desired composed resource %q", name))
			return rsp, nil
		}

		labels := obj.GetLabels()
		if labels == nil {
			labels = make(map[string]string, len(in.Labels))
		}
		for k, v := range in.Labels {
			labels[k] = v
		}
		obj.SetLabels(labels)

		s, err := resource.AsStruct(obj)
		if err != nil {
			response.Fatal(rsp, errors.Wrapf(err, "cannot write desired composed resource %q", name))
			return rsp, nil
		}
		r.Resource = s
	}

	response.Normalf(rsp, "labelled %d composed resource(s)", len(names))
	return rsp, nil
}

// validateLabels rejects an empty set and anything Kubernetes would refuse as a label, so a typo fails the
// pipeline here with a clear message instead of failing later when the resource is applied.
func validateLabels(labels map[string]string) error {
	if len(labels) == 0 {
		return errors.New("input.labels must contain at least one label")
	}

	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		if errs := validation.IsQualifiedName(k); len(errs) > 0 {
			return errors.Errorf("invalid label key %q: %s", k, strings.Join(errs, "; "))
		}
		if errs := validation.IsValidLabelValue(labels[k]); len(errs) > 0 {
			return errors.Errorf("invalid value %q for label %q: %s", labels[k], k, strings.Join(errs, "; "))
		}
	}
	return nil
}
