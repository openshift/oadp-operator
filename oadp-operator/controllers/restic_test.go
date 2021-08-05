package controllers

import (
	"context"
	"reflect"
	"testing"

	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestVeleroReconciler_ReconcileResticDaemonset(t *testing.T) {
	type fields struct {
		Client         client.Client
		Scheme         *runtime.Scheme
		Log            logr.Logger
		Context        context.Context
		NamespacedName types.NamespacedName
		EventRecorder  record.EventRecorder
	}
	type args struct {
		log logr.Logger
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    bool
		wantErr bool
	}{
		//TODO: Add tests
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &VeleroReconciler{
				Client:         tt.fields.Client,
				Scheme:         tt.fields.Scheme,
				Log:            tt.fields.Log,
				Context:        tt.fields.Context,
				NamespacedName: tt.fields.NamespacedName,
				EventRecorder:  tt.fields.EventRecorder,
			}
			got, err := r.ReconcileResticDaemonset(tt.args.log)
			if (err != nil) != tt.wantErr {
				t.Errorf("VeleroReconciler.ReconcileResticDaemonset() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("VeleroReconciler.ReconcileResticDaemonset() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVeleroReconciler_buildResticDaemonset(t *testing.T) {
	type fields struct {
		Client         client.Client
		Scheme         *runtime.Scheme
		Log            logr.Logger
		Context        context.Context
		NamespacedName types.NamespacedName
		EventRecorder  record.EventRecorder
	}
	type args struct {
		velero *oadpv1alpha1.Velero
		ds     *appsv1.DaemonSet
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *appsv1.DaemonSet
		wantErr bool
	}{
		{
			name:   "velero is nil",
			fields: fields{NamespacedName: types.NamespacedName{Namespace: "velero"}},
			args: args{
				nil, &appsv1.DaemonSet{},
			},
			wantErr: true,
			want:    nil,
		},
		{
			name: "DaemonSet is nil",
			args: args{
				&oadpv1alpha1.Velero{}, nil,
			},
			wantErr: true,
			want:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &VeleroReconciler{
				Client:         tt.fields.Client,
				Scheme:         tt.fields.Scheme,
				Log:            tt.fields.Log,
				Context:        tt.fields.Context,
				NamespacedName: tt.fields.NamespacedName,
				EventRecorder:  tt.fields.EventRecorder,
			}
			got, err := r.buildResticDaemonset(tt.args.velero, tt.args.ds)
			if (err != nil) != tt.wantErr {
				t.Errorf("VeleroReconciler.buildResticDaemonset() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("VeleroReconciler.buildResticDaemonset() = %v, want %v", got, tt.want)
			}
		})
	}
}
