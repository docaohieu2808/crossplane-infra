package main

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/crossplane/function-sdk-go/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
)

// labelsOf returns metadata.labels of the named desired composed resource in a response.
func labelsOf(t *testing.T, rsp *fnv1.RunFunctionResponse, name string) map[string]any {
	t.Helper()
	r, ok := rsp.GetDesired().GetResources()[name]
	if !ok {
		t.Fatalf("response has no desired resource %q", name)
	}
	md, _ := r.GetResource().AsMap()["metadata"].(map[string]any)
	labels, _ := md["labels"].(map[string]any)
	return labels
}

func TestRunFunction(t *testing.T) {
	input := `{
		"apiVersion": "resourcelabeler.fn.example.io/v1beta1",
		"kind": "Input",
		"labels": {"app.kubernetes.io/managed-by": "crossplane", "team": "shop"}
	}`

	cases := map[string]struct {
		reason string
		req    *fnv1.RunFunctionRequest
		// wantLabels: resource name -> expected metadata.labels (nil map = the resource must have none).
		wantLabels map[string]map[string]any
		// wantSeverity of the single result the function must return.
		wantSeverity fnv1.Severity
	}{
		"LabelsEveryComposedResource": {
			reason: "Every desired composed resource gets the input labels; labels set by an earlier step survive",
			req: &fnv1.RunFunctionRequest{
				Meta:  &fnv1.RequestMeta{Tag: "t"},
				Input: resource.MustStructJSON(input),
				Desired: &fnv1.State{Resources: map[string]*fnv1.Resource{
					"vpc": {Resource: resource.MustStructJSON(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"a","labels":{"environment":"dev"}}}`)},
					"vm":  {Resource: resource.MustStructJSON(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"b"}}`)},
				}},
			},
			wantLabels: map[string]map[string]any{
				"vpc": {"environment": "dev", "app.kubernetes.io/managed-by": "crossplane", "team": "shop"},
				"vm":  {"app.kubernetes.io/managed-by": "crossplane", "team": "shop"},
			},
			wantSeverity: fnv1.Severity_SEVERITY_NORMAL,
		},
		"InputWinsOnConflict": {
			reason: "A label with the same key as one from an earlier step takes the input value",
			req: &fnv1.RunFunctionRequest{
				Input: resource.MustStructJSON(input),
				Desired: &fnv1.State{Resources: map[string]*fnv1.Resource{
					"vpc": {Resource: resource.MustStructJSON(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"a","labels":{"team":"someone-else"}}}`)},
				}},
			},
			wantLabels: map[string]map[string]any{
				"vpc": {"app.kubernetes.io/managed-by": "crossplane", "team": "shop"},
			},
			wantSeverity: fnv1.Severity_SEVERITY_NORMAL,
		},
		"NothingToLabel": {
			reason: "No desired resources is not an error",
			req: &fnv1.RunFunctionRequest{
				Input: resource.MustStructJSON(input),
			},
			wantSeverity: fnv1.Severity_SEVERITY_NORMAL,
		},
		"MissingLabels": {
			reason: "An input without labels is a fatal result",
			req: &fnv1.RunFunctionRequest{
				Input: resource.MustStructJSON(`{"apiVersion":"resourcelabeler.fn.example.io/v1beta1","kind":"Input"}`),
			},
			wantSeverity: fnv1.Severity_SEVERITY_FATAL,
		},
		"InvalidLabelKey": {
			reason: "A label key Kubernetes would reject is a fatal result",
			req: &fnv1.RunFunctionRequest{
				Input: resource.MustStructJSON(`{"apiVersion":"resourcelabeler.fn.example.io/v1beta1","kind":"Input","labels":{"not a key":"x"}}`),
			},
			wantSeverity: fnv1.Severity_SEVERITY_FATAL,
		},
		"InvalidLabelValue": {
			reason: "A label value Kubernetes would reject is a fatal result",
			req: &fnv1.RunFunctionRequest{
				Input: resource.MustStructJSON(`{"apiVersion":"resourcelabeler.fn.example.io/v1beta1","kind":"Input","labels":{"team":"has spaces"}}`),
			},
			wantSeverity: fnv1.Severity_SEVERITY_FATAL,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := &Function{log: logging.NewNopLogger()}
			rsp, err := f.RunFunction(context.Background(), tc.req)
			if err != nil {
				t.Fatalf("%s\nf.RunFunction(...): unexpected error: %v", tc.reason, err)
			}

			if got := len(rsp.GetResults()); got != 1 {
				t.Fatalf("%s\nf.RunFunction(...): want 1 result, got %d: %v", tc.reason, got, rsp.GetResults())
			}
			if got := rsp.GetResults()[0].GetSeverity(); got != tc.wantSeverity {
				t.Errorf("%s\nf.RunFunction(...): want severity %s, got %s (%s)", tc.reason, tc.wantSeverity, got, rsp.GetResults()[0].GetMessage())
			}

			for res, want := range tc.wantLabels {
				if diff := cmp.Diff(want, labelsOf(t, rsp, res)); diff != "" {
					t.Errorf("%s\nlabels of %q: -want, +got:\n%s", tc.reason, res, diff)
				}
			}
		})
	}
}

func TestRunFunctionPreservesReadinessAndConnectionDetails(t *testing.T) {
	req := &fnv1.RunFunctionRequest{
		Input: resource.MustStructJSON(`{"apiVersion":"resourcelabeler.fn.example.io/v1beta1","kind":"Input","labels":{"team":"shop"}}`),
		Desired: &fnv1.State{Resources: map[string]*fnv1.Resource{
			"db": {
				Resource:          resource.MustStructJSON(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"db"}}`),
				Ready:             fnv1.Ready_READY_TRUE,
				ConnectionDetails: map[string][]byte{"password": []byte("hunter2")},
			},
		}},
	}
	want := &fnv1.Resource{
		Resource:          resource.MustStructJSON(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"db","labels":{"team":"shop"}}}`),
		Ready:             fnv1.Ready_READY_TRUE,
		ConnectionDetails: map[string][]byte{"password": []byte("hunter2")},
	}

	f := &Function{log: logging.NewNopLogger()}
	rsp, err := f.RunFunction(context.Background(), req)
	if err != nil {
		t.Fatalf("f.RunFunction(...): unexpected error: %v", err)
	}

	if diff := cmp.Diff(want, rsp.GetDesired().GetResources()["db"], protocmp.Transform()); diff != "" {
		t.Errorf("readiness and connection details must survive labelling: -want, +got:\n%s", diff)
	}
}
