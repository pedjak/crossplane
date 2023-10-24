/*
Copyright 2020 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package claim

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/resource/fake"
	"github.com/crossplane/crossplane-runtime/pkg/resource/unstructured/claim"
	"github.com/crossplane/crossplane-runtime/pkg/resource/unstructured/composite"
	"github.com/crossplane/crossplane-runtime/pkg/test"

	"github.com/crossplane/crossplane/internal/xcrd"
)

func TestCompositeConfigure(t *testing.T) {
	ns := "spacename"
	name := "cool"
	apiVersion := "v"
	kind := "k"
	now := metav1.Now()

	type args struct {
		ctx context.Context
		cm  resource.CompositeClaim
		cp  resource.Composite
	}

	type want struct {
		cp  resource.Composite
		err error
	}

	cases := map[string]struct {
		reason      string
		diffOptions []cmp.Option
		extraAssert func(args args, patchCp *composite.Unstructured, t *testing.T)
		args        args
		want        want
	}{
		"ClaimNotUnstructured": {
			reason: "We should return early if the claim is not unstructured",
			args: args{
				cm: &fake.CompositeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Name:      name,
					},
				},
				cp: &fake.Composite{},
			},
			want: want{
				cp: &composite.Unstructured{},
			},
		},
		"CompositeNotUnstructured": {
			reason: "We should return early if the composite is not unstructured",
			args: args{
				cm: &claim.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"metadata": map[string]any{
								"namespace": ns,
								"name":      name,
							},
						},
					},
				},
				cp: &fake.Composite{},
			},
			want: want{
				cp: &composite.Unstructured{},
			},
		},
		"UnsupportedSpecError": {
			reason: "We should return early if the claim's spec is not an unstructured object",
			args: args{
				cm: &claim.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"metadata": map[string]any{
								"namespace": ns,
								"name":      name,
							},
							"spec": "wat",
						},
					},
				},
				cp: &composite.Unstructured{},
			},
			want: want{
				cp:  &composite.Unstructured{},
				err: errors.New(errUnsupportedClaimSpec),
			},
		},
		"AlreadyClaimedError": {
			reason: "We should return an error if we appear to be configuring a composite resource claimed by a different... claim.",
			args: args{
				cm: &claim.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"metadata": map[string]any{
								"namespace": ns,
								"name":      name,
							},
							"spec": map[string]any{},
						},
					},
				},
				cp: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"spec": map[string]any{
								"claimRef": map[string]any{
									"apiVersion": apiVersion,
									"kind":       kind,
									"namespace":  ns,
									"name":       "some-other-claim",
								},
							},
						},
					},
				},
			},
			want: want{
				cp: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"spec": map[string]any{
								"claimRef": map[string]any{
									"apiVersion": apiVersion,
									"kind":       kind,
									"namespace":  ns,
									"name":       "some-other-claim",
								},
							},
						},
					},
				},
				err: ErrBindCompositeConflict,
			},
		},
		"ConfiguredNewXR": {
			reason: "A dynamically provisioned composite resource should be configured according to the claim",
			args: args{
				cm: &claim.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"apiVersion": apiVersion,
							"kind":       kind,
							"metadata": map[string]any{
								"namespace": ns,
								"name":      name,
							},
							"spec": map[string]any{
								"coolness": 23,

								// These should be preserved.
								"compositionRef":      "ref",
								"compositionSelector": "ref",

								// These should be filtered out.
								"resourceRef":                "ref",
								"writeConnectionSecretToRef": "ref",
							},
						},
					},
				},
				cp: &composite.Unstructured{},
			},
			diffOptions: []cmp.Option{
				cmp.FilterPath(
					func(path cmp.Path) bool {
						// ignore metadata.name because it is generated with random suffix
						return strings.HasPrefix(path.GoString(), "{*composite.Unstructured}.Unstructured.Object[\"metadata\"].(map[string]any)[\"name\"]")
					},
					cmp.Ignore()),
			},
			extraAssert: func(args args, patchCp *composite.Unstructured, t *testing.T) {

				cpname := patchCp.GetName()
				cmname := args.cm.GetName()
				prefix := fmt.Sprintf("%s-", cmname)
				fmt.Println(cpname)
				if !strings.HasPrefix(cpname, prefix) {
					t.Errorf("composite name %s must be based on claim name %s", cpname, cmname)
				}
				l := len(prefix) + 5
				if len(cpname) != l {
					t.Errorf("composite name %s should be of length %v", cpname, l)
				}
				suffix := cpname[len(prefix):]
				if l := len(suffix); l != 5 {
					t.Errorf("composite name %s suffix must of length 5 got: %v", cpname, l)
				}
			},
			want: want{
				cp: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"metadata": map[string]any{
								// "name" has a random suffix, hence we cannot make a stable diff
								"labels": map[string]any{
									xcrd.LabelKeyClaimNamespace: ns,
									xcrd.LabelKeyClaimName:      name,
								},
							},
							"spec": map[string]any{
								"coolness":            23,
								"compositionRef":      "ref",
								"compositionSelector": "ref",
								"claimRef": map[string]any{
									"apiVersion": apiVersion,
									"kind":       kind,
									"namespace":  ns,
									"name":       name,
								},
							},
						},
					},
				},
			},
		},
		"ConfiguredExistingXR": {
			reason: "A statically provisioned composite resource should be configured according to the claim",
			args: args{
				cm: &claim.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"apiVersion": apiVersion,
							"kind":       kind,
							"metadata": map[string]any{
								"namespace": ns,
								"name":      name,
								"annotations": map[string]any{
									// This should be reset to the equivalent
									// composite resource value, since it has
									// most likely already taken effect and
									// cannot be updated retroactively.
									meta.AnnotationKeyExternalName: "wat",
									"xrc":                          "annotation",
								},
							},
							"spec": map[string]any{
								"coolness": 23,

								// These should be filtered out.
								"resourceRef":                "ref",
								"writeConnectionSecretToRef": "ref",
							},
						},
					},
				},
				cp: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"metadata": map[string]any{
								"name": name,
								"creationTimestamp": func() string {
									b, _ := now.MarshalJSON()
									return strings.Trim(string(b), "\"")
								}(),
								"labels": map[string]any{
									xcrd.LabelKeyClaimNamespace: ns,
									xcrd.LabelKeyClaimName:      name,
								},
								"annotations": map[string]any{
									meta.AnnotationKeyExternalName: name,
									"xr":                           "annotation",
								},
							},
							"spec": map[string]any{
								// This should be overridden with the value of
								// the equivalent claim field.
								"coolness": 42,
							},
						},
					},
				},
			},
			want: want{
				cp: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"metadata": map[string]any{
								"name": name,
								"labels": map[string]any{
									xcrd.LabelKeyClaimNamespace: ns,
									xcrd.LabelKeyClaimName:      name,
								},
								"annotations": map[string]any{
									meta.AnnotationKeyExternalName: name,
									"xrc":                          "annotation",
								},
							},
							"spec": map[string]any{
								"coolness": 23,
								"claimRef": map[string]any{
									"apiVersion": apiVersion,
									"kind":       kind,
									"namespace":  ns,
									"name":       name,
								},
							},
						},
					},
				},
			},
		},
		"UpdatePolicyAutomatic": {
			reason: "CompositionRevision of composite should NOT be set by the claim",
			args: args{
				cm: &claim.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"apiVersion": apiVersion,
							"kind":       kind,
							"metadata": map[string]any{
								"namespace": ns,
								"name":      name,
							},
							"spec": map[string]any{
								"resourceRef":                "ref",
								"writeConnectionSecretToRef": "ref",
								"compositionUpdatePolicy":    "Automatic",
								"compositionRevisionRef": map[string]any{
									"name": "newref",
								},
							},
							"status": map[string]any{
								"previousCoolness": 23,
								"conditions": []map[string]any{
									{
										"type": "someCondition",
									},
								},
							},
						},
					},
				},
				cp: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"metadata": map[string]any{
								"namespace": ns,
								"name":      name + "-12345",
								"creationTimestamp": func() string {
									b, _ := now.MarshalJSON()
									return strings.Trim(string(b), "\"")
								}(),
								"labels": map[string]any{
									xcrd.LabelKeyClaimNamespace: ns,
									xcrd.LabelKeyClaimName:      name,
								},
							},
							"spec": map[string]any{
								"compositionUpdatePolicy": "Automatic",
								"claimRef": map[string]any{
									"apiVersion": apiVersion,
									"kind":       kind,
									"namespace":  ns,
									"name":       name,
								},
							},
							"status": map[string]any{
								"previousCoolness": 28,
								"conditions": []map[string]any{
									{
										"type": "otherCondition",
									},
								},
							},
						},
					},
				},
			},
			want: want{
				cp: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"metadata": map[string]any{
								"name": name + "-12345",
								"labels": map[string]any{
									xcrd.LabelKeyClaimNamespace: ns,
									xcrd.LabelKeyClaimName:      name,
								},
							},
							"spec": map[string]any{
								"compositionUpdatePolicy": "Automatic",
								"claimRef": map[string]any{
									"apiVersion": apiVersion,
									"kind":       kind,
									"namespace":  ns,
									"name":       name,
								},
							},
						},
					},
				},
			},
		},
		"UpdatePolicyManual": {
			reason: "CompositionRevision of composite should be overwritten by the claim",
			args: args{
				cm: &claim.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"apiVersion": apiVersion,
							"kind":       kind,
							"metadata": map[string]any{
								"namespace": ns,
								"name":      name,
							},
							"spec": map[string]any{
								"resourceRef":                "ref",
								"writeConnectionSecretToRef": "ref",
								"compositionUpdatePolicy":    "Manual",
								"compositionRevisionRef": map[string]any{
									"name": "newref",
								},
							},
							"status": map[string]any{
								"previousCoolness": 23,
								"conditions": []map[string]any{
									{
										"type": "someCondition",
									},
								},
							},
						},
					},
				},
				cp: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"metadata": map[string]any{
								"namespace": ns,
								"name":      name + "-12345",
								"creationTimestamp": func() string {
									b, _ := now.MarshalJSON()
									return strings.Trim(string(b), "\"")
								}(),
								"labels": map[string]any{
									xcrd.LabelKeyClaimNamespace: ns,
									xcrd.LabelKeyClaimName:      name,
								},
							},
							"spec": map[string]any{
								"compositionUpdatePolicy": "Manual",
								"compositionRevisionRef": map[string]any{
									"name": "oldref",
								},
								"claimRef": map[string]any{
									"apiVersion": apiVersion,
									"kind":       kind,
									"namespace":  ns,
									"name":       name,
								},
							},
							"status": map[string]any{
								"previousCoolness": 28,
								"conditions": []map[string]any{
									{
										"type": "otherCondition",
									},
								},
							},
						},
					},
				},
			},
			want: want{
				cp: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"metadata": map[string]any{
								"name": name + "-12345",
								"labels": map[string]any{
									xcrd.LabelKeyClaimNamespace: ns,
									xcrd.LabelKeyClaimName:      name,
								},
							},
							"spec": map[string]any{
								"compositionUpdatePolicy": "Manual",
								"compositionRevisionRef": map[string]any{
									"name": "newref",
								},
								"claimRef": map[string]any{
									"apiVersion": apiVersion,
									"kind":       kind,
									"namespace":  ns,
									"name":       name,
								},
							},
						},
					},
				},
			},
		},
		"SkipK8sAnnotationPropagation": {
			reason: "Claim's kubernetes.io annotations should not be propagated to XR",
			args: args{
				cm: &claim.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"apiVersion": apiVersion,
							"kind":       kind,
							"metadata": map[string]any{
								"namespace": ns,
								"name":      name,
								"annotations": map[string]any{
									"foo":                   "v1",
									"foo.kubernetes.io":     "v2",
									"foo.kubernetes.io/bar": "v3",
									"foo.k8s.io":            "v4",
									"foo.k8s.io/bar":        "v5",
								},
							},
							"spec": map[string]any{
								"coolness": 23,

								// These should be preserved.
								"compositionRef":      "ref",
								"compositionSelector": "ref",

								// These should be filtered out.
								"resourceRef":                "ref",
								"writeConnectionSecretToRef": "ref",
							},
						},
					},
				},
				cp: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]interface{}{
							"metadata": map[string]any{
								"creationTimestamp": func() string {
									b, _ := now.MarshalJSON()
									return strings.Trim(string(b), "\"")
								}(),
							},
						},
					},
				},
			},
			want: want{
				cp: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"metadata": map[string]any{
								"labels": map[string]any{
									xcrd.LabelKeyClaimNamespace: ns,
									xcrd.LabelKeyClaimName:      name,
								},
								"annotations": map[string]any{
									"foo": "v1",
								},
							},
							"spec": map[string]any{
								"coolness":            23,
								"compositionRef":      "ref",
								"compositionSelector": "ref",
								"claimRef": map[string]any{
									"apiVersion": apiVersion,
									"kind":       kind,
									"namespace":  ns,
									"name":       name,
								},
							},
						},
					},
				},
			},
		},
		"SkipK8sLabelPropagation": {
			reason: "Claim's kubernetes.io annotations should not be propagated to XR",
			args: args{
				cm: &claim.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"apiVersion": apiVersion,
							"kind":       kind,
							"metadata": map[string]any{
								"namespace": ns,
								"name":      name,
								"labels": map[string]any{
									"foo":                   "v1",
									"foo.kubernetes.io":     "v2",
									"foo.kubernetes.io/bar": "v3",
									"foo.k8s.io":            "v4",
									"foo.k8s.io/bar":        "v5",
								},
							},
							"spec": map[string]any{
								"coolness": 23,

								// These should be preserved.
								"compositionRef":      "ref",
								"compositionSelector": "ref",

								// These should be filtered out.
								"resourceRef":                "ref",
								"writeConnectionSecretToRef": "ref",
							},
						},
					},
				},
				cp: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]interface{}{
							"metadata": map[string]any{
								"creationTimestamp": func() string {
									b, _ := now.MarshalJSON()
									return strings.Trim(string(b), "\"")
								}(),
							},
						},
					}},
			},
			want: want{
				cp: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"metadata": map[string]any{
								"labels": map[string]any{
									xcrd.LabelKeyClaimNamespace: ns,
									xcrd.LabelKeyClaimName:      name,
									"foo":                       "v1",
								},
							},
							"spec": map[string]any{
								"coolness":            23,
								"compositionRef":      "ref",
								"compositionSelector": "ref",
								"claimRef": map[string]any{
									"apiVersion": apiVersion,
									"kind":       kind,
									"namespace":  ns,
									"name":       name,
								},
							},
						},
					},
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			patchCp := &composite.Unstructured{}
			got := configureComposite(tc.args.ctx, tc.args.cm, tc.args.cp, patchCp)
			if diff := cmp.Diff(tc.want.err, got, test.EquateErrors()); diff != "" {
				t.Errorf("Configure(...): %s\n-want error, +got error:\n%s\n", tc.reason, diff)
			}
			if tc.want.err == nil {
				if diff := cmp.Diff(tc.want.cp, patchCp, tc.diffOptions...); diff != "" {
					t.Errorf("Configure(...): %s\n-want, +got:\n%s\n", tc.reason, diff)
				}
			}

			if tc.extraAssert != nil {
				tc.extraAssert(tc.args, patchCp, t)
			}
		})
	}

}

func TestClaimConfigure(t *testing.T) {
	ns := "spacename"
	name := "cool"

	type args struct {
		cm     resource.CompositeClaim
		cp     resource.Composite
		client client.Client
	}

	type want struct {
		cm  resource.CompositeClaim
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"MergeStatusClaimError": {
			reason: "Should return an error if unable to merge status of claim",
			args: args{
				cm: &claim.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"status": "notStatus",
						},
					},
				},
				cp: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"status": "notStatus",
						},
					},
				},
			},
			want: want{
				cm: &claim.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"status": "notStatus",
						},
					},
				},
				err: errors.Wrap(errors.New(errUnsupportedSrcObject), errMergeClaimStatus),
			},
		},
		"MergeStatusCompositeError": {
			reason: "Should return an error if unable to merge status from composite",
			args: args{
				cm: &claim.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"status": map[string]any{},
						},
					},
				},
				cp: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"status": "notStatus",
						},
					},
				},
			},
			want: want{
				err: errors.Wrap(errors.New(errUnsupportedSrcObject), errMergeClaimStatus),
			},
		},
		"MergeSpecError": {
			reason: "Should return an error if unable to merge spec",
			args: args{
				client: &test.MockClient{
					MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
				},
				cm: &claim.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"spec":   "notSpec",
							"status": map[string]any{},
						},
					},
				},
				cp: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"spec":   "notSpec",
							"status": map[string]any{},
						},
					},
				},
			},
			want: want{
				cm: &claim.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"spec":   "notSpec",
							"status": map[string]any{},
						},
					},
				},
				err: errors.Wrap(errors.New(errUnsupportedSrcObject), errMergeClaimSpec),
			},
		},
		"LateInitializeClaim": {
			reason: "Empty fields in claim should be late initialized from the composite",
			args: args{
				client: test.NewMockClient(),
				cm: &claim.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"metadata": map[string]any{
								"namespace": ns,
								"name":      name,
								"annotations": map[string]any{
									meta.AnnotationKeyExternalName: "nope",
								},
							},
							"spec": map[string]any{
								"someField":                  "someValue",
								"resourceRef":                "ref",
								"writeConnectionSecretToRef": "ref",
							},
							"status": map[string]any{},
						},
					},
				},
				cp: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"metadata": map[string]any{
								"name": name + "-12345",
								"annotations": map[string]any{
									meta.AnnotationKeyExternalName: name,
								},
							},
							"spec": map[string]any{
								// This user-defined field should be propagated.
								"coolness": 23,

								// These machinery fields should be propagated.
								"compositionSelector":     "sel",
								"compositionRef":          "ref",
								"compositionUpdatePolicy": "pol",

								// These should be filtered out.
								"resourceRefs": "ref",
								"claimRef":     "ref",
							},
							"status": map[string]any{},
						},
					},
				},
			},
			want: want{
				cm: &claim.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"metadata": map[string]any{
								"annotations": map[string]any{
									meta.AnnotationKeyExternalName: name,
								},
							},
							"spec": map[string]any{
								"resourceRef":             map[string]any{"name": "cool-12345"},
								"compositionSelector":     "sel",
								"compositionRef":          "ref",
								"compositionUpdatePolicy": "pol",
							},
							"status": map[string]any{},
						},
					},
				},
			},
		},
		"ConfigureStatus": {
			reason: "Status of claim should be overwritten by the composite",
			args: args{
				client: test.NewMockClient(),
				cm: &claim.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"metadata": map[string]any{
								"namespace": ns,
								"name":      name,
							},
							"spec": map[string]any{
								"resourceRef":                "ref",
								"writeConnectionSecretToRef": "ref",
							},
							"status": map[string]any{
								"previousCoolness": 23,
								"conditions": []map[string]any{
									{
										"type": "someCondition",
									},
								},
							},
						},
					},
				},
				cp: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"metadata": map[string]any{
								"name": name + "-12345",
							},
							"spec": map[string]any{
								"resourceRefs": "ref",
								"claimRef":     "ref",
							},
							"status": map[string]any{
								"previousCoolness": 28,
								"conditions": []map[string]any{
									{
										"type": "otherCondition",
									},
								},
							},
						},
					},
				},
			},
			want: want{
				cm: &claim.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"spec": map[string]any{
								"resourceRef": map[string]any{"name": string("cool-12345")},
							},
							"status": map[string]any{
								"previousCoolness": 28,
								"conditions": []map[string]any{
									{
										"type": "someCondition",
									},
								},
							},
						},
					},
				},
			},
		},
		"UpdatePolicyManual": {
			reason: "CompositionRevision of claim should NOT overwritten by the composite",
			args: args{
				client: test.NewMockClient(),
				cm: &claim.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"metadata": map[string]any{
								"namespace": ns,
								"name":      name,
							},
							"spec": map[string]any{
								"resourceRef":                "ref",
								"writeConnectionSecretToRef": "ref",
								"compositionUpdatePolicy":    "Manual",
								"compositionRevisionRef": map[string]any{
									"name": "oldref",
								},
							},
							"status": map[string]any{
								"previousCoolness": 23,
								"conditions": []map[string]any{
									{
										"type": "someCondition",
									},
								},
							},
						},
					},
				},
				cp: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"metadata": map[string]any{
								"name": name + "-12345",
							},
							"spec": map[string]any{
								"resourceRefs":            "ref",
								"claimRef":                "ref",
								"compositionUpdatePolicy": "Manual",
								"compositionRevisionRef": map[string]any{
									"name": "newref",
								},
							},
							"status": map[string]any{
								"previousCoolness": 28,
								"conditions": []map[string]any{
									{
										"type": "otherCondition",
									},
								},
							},
						},
					},
				},
			},
			want: want{
				cm: &claim.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"spec": map[string]any{
								"resourceRef":             map[string]any{"name": string("cool-12345")},
								"compositionUpdatePolicy": "Manual",
							},
							"status": map[string]any{
								"previousCoolness": 28,
								"conditions": []map[string]any{
									{
										"type": "someCondition",
									},
								},
							},
						},
					},
				},
			},
		},
		"UpdatePolicyAutomatic": {
			reason: "CompositionRevision of claim should be overwritten by the composite",
			args: args{
				client: test.NewMockClient(),
				cm: &claim.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"metadata": map[string]any{
								"namespace": ns,
								"name":      name,
							},
							"spec": map[string]any{
								"resourceRef":                "ref",
								"writeConnectionSecretToRef": "ref",
								"compositionUpdatePolicy":    "Automatic",
								"compositionRevisionRef": map[string]any{
									"name": "oldref",
								},
							},
							"status": map[string]any{
								"previousCoolness": 23,
								"conditions": []map[string]any{
									{
										"type": "someCondition",
									},
								},
							},
						},
					},
				},
				cp: &composite.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"metadata": map[string]any{
								"name": name + "-12345",
							},
							"spec": map[string]any{
								"resourceRefs":            "ref",
								"claimRef":                "ref",
								"compositionUpdatePolicy": "Automatic",
								"compositionRevisionRef": map[string]any{
									"name": "newref",
								},
							},
							"status": map[string]any{
								"previousCoolness": 28,
								"conditions": []map[string]any{
									{
										"type": "otherCondition",
									},
								},
							},
						},
					},
				},
			},
			want: want{
				cm: &claim.Unstructured{
					Unstructured: unstructured.Unstructured{
						Object: map[string]any{
							"spec": map[string]any{
								"resourceRef":             map[string]any{"name": string("cool-12345")},
								"compositionUpdatePolicy": "Automatic",
								"compositionRevisionRef": map[string]any{
									"name": "newref",
								},
							},
							"status": map[string]any{
								"previousCoolness": 28,
								"conditions": []map[string]any{
									{
										"type": "someCondition",
									},
								},
							},
						},
					},
				},
			},
		},
		"CompositeRefConflict": {
			reason: "An error should be returned if the claim is bound to another composite resource",
			args: args{
				cm: &fake.CompositeClaim{
					CompositeResourceReferencer: fake.CompositeResourceReferencer{
						Ref: &corev1.ObjectReference{
							Name: "who",
						},
					},
				},
				cp: &fake.Composite{
					ObjectMeta: metav1.ObjectMeta{
						Name: "wat",
					},
				},
			},
			want: want{
				cm: &fake.CompositeClaim{
					CompositeResourceReferencer: fake.CompositeResourceReferencer{
						Ref: &corev1.ObjectReference{
							Name: "who",
						},
					},
				},
				err: errors.New(errBindClaimConflict),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			patchCm := claim.New()
			got := configureClaim(context.Background(), tc.args.cm, patchCm, tc.args.cp, tc.args.cp)
			if diff := cmp.Diff(tc.want.err, got, test.EquateErrors()); diff != "" {
				t.Errorf("c.Configure(...): %s\n-want error, +got error:\n%s\n", tc.reason, diff)
			}
			if tc.want.err == nil {
				if diff := cmp.Diff(tc.want.cm, patchCm); diff != "" {
					t.Errorf("c.Configure(...): %s\n-want, +got:\n%s\n", tc.reason, diff)
				}
			}
		})
	}

}
