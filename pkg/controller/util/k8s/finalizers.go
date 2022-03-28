// SPDX-FileCopyrightText: 2019-present Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package k8s

// HasFinalizer returns whether the given set of finalizers includes finalizer name
func HasFinalizer(finalizers []string, name string) bool {
	for _, finalizer := range finalizers {
		if finalizer == name {
			return true
		}
	}
	return false
}

// AddFinalizer adds the given finalizer to the set of finalizers
func AddFinalizer(finalizers []string, name string) []string {
	return append(finalizers, name)
}

// RemoveFinalizer remopves the given finalizer from the set of finalizers
func RemoveFinalizer(finalizers []string, name string) []string {
	newFinalizers := make([]string, 0, len(finalizers))
	for _, finalizer := range finalizers {
		if finalizer != name {
			newFinalizers = append(newFinalizers, finalizer)
		}
	}
	return newFinalizers
}
