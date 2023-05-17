//go:build !ignore_autogenerated
// +build !ignore_autogenerated

/*
Copyright 2021.

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

// Code generated by controller-gen. DO NOT EDIT.

package server

import (
	timex "time"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Args) DeepCopyInto(out *Args) {
	*out = *in
	in.ServerConfig.DeepCopyInto(&out.ServerConfig)
	in.GlobalFlags.DeepCopyInto(&out.GlobalFlags)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Args.
func (in *Args) DeepCopy() *Args {
	if in == nil {
		return nil
	}
	out := new(Args)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GlobalFlags) DeepCopyInto(out *GlobalFlags) {
	*out = *in
	in.VeleroConfig.DeepCopyInto(&out.VeleroConfig)
	in.LoggingT.DeepCopyInto(&out.LoggingT)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GlobalFlags.
func (in *GlobalFlags) DeepCopy() *GlobalFlags {
	if in == nil {
		return nil
	}
	out := new(GlobalFlags)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ServerConfig) DeepCopyInto(out *ServerConfig) {
	*out = *in
	if in.BackupSyncPeriod != nil {
		in, out := &in.BackupSyncPeriod, &out.BackupSyncPeriod
		*out = new(timex.Duration)
		**out = **in
	}
	if in.PodVolumeOperationTimeout != nil {
		in, out := &in.PodVolumeOperationTimeout, &out.PodVolumeOperationTimeout
		*out = new(timex.Duration)
		**out = **in
	}
	if in.ResourceTerminatingTimeout != nil {
		in, out := &in.ResourceTerminatingTimeout, &out.ResourceTerminatingTimeout
		*out = new(timex.Duration)
		**out = **in
	}
	if in.DefaultBackupTTL != nil {
		in, out := &in.DefaultBackupTTL, &out.DefaultBackupTTL
		*out = new(timex.Duration)
		**out = **in
	}
	if in.StoreValidationFrequency != nil {
		in, out := &in.StoreValidationFrequency, &out.StoreValidationFrequency
		*out = new(timex.Duration)
		**out = **in
	}
	if in.DisabledControllers != nil {
		in, out := &in.DisabledControllers, &out.DisabledControllers
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.ClientQPS != nil {
		in, out := &in.ClientQPS, &out.ClientQPS
		*out = new(string)
		**out = **in
	}
	if in.ClientBurst != nil {
		in, out := &in.ClientBurst, &out.ClientBurst
		*out = new(int)
		**out = **in
	}
	if in.ClientPageSize != nil {
		in, out := &in.ClientPageSize, &out.ClientPageSize
		*out = new(int)
		**out = **in
	}
	if in.ItemOperationSyncFrequency != nil {
		in, out := &in.ItemOperationSyncFrequency, &out.ItemOperationSyncFrequency
		*out = new(timex.Duration)
		**out = **in
	}
	if in.RepoMaintenanceFrequency != nil {
		in, out := &in.RepoMaintenanceFrequency, &out.RepoMaintenanceFrequency
		*out = new(timex.Duration)
		**out = **in
	}
	if in.GarbageCollectionFrequency != nil {
		in, out := &in.GarbageCollectionFrequency, &out.GarbageCollectionFrequency
		*out = new(timex.Duration)
		**out = **in
	}
	if in.DefaultVolumesToFsBackup != nil {
		in, out := &in.DefaultVolumesToFsBackup, &out.DefaultVolumesToFsBackup
		*out = new(bool)
		**out = **in
	}
	if in.DefaultItemOperationTimeout != nil {
		in, out := &in.DefaultItemOperationTimeout, &out.DefaultItemOperationTimeout
		*out = new(timex.Duration)
		**out = **in
	}
	if in.ResourceTimeout != nil {
		in, out := &in.ResourceTimeout, &out.ResourceTimeout
		*out = new(timex.Duration)
		**out = **in
	}
	if in.MaxConcurrentK8SConnections != nil {
		in, out := &in.MaxConcurrentK8SConnections, &out.MaxConcurrentK8SConnections
		*out = new(int)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ServerConfig.
func (in *ServerConfig) DeepCopy() *ServerConfig {
	if in == nil {
		return nil
	}
	out := new(ServerConfig)
	in.DeepCopyInto(out)
	return out
}
