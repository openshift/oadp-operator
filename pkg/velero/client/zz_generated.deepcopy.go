//go:build !ignore_autogenerated

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

package client

import ()

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VeleroConfig) DeepCopyInto(out *VeleroConfig) {
	*out = *in
	if in.Colorized != nil {
		in, out := &in.Colorized, &out.Colorized
		*out = new(bool)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VeleroConfig.
func (in *VeleroConfig) DeepCopy() *VeleroConfig {
	if in == nil {
		return nil
	}
	out := new(VeleroConfig)
	in.DeepCopyInto(out)
	return out
}
