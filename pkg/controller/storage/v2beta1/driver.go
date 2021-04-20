// Copyright 2019-present Open Networking Foundation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v2beta1

import (
	"fmt"
	"os"
	"strconv"
)

const (
	driverAnnotationFormat = "%s.storage.atomix.io/%s"
	injectAnnotation       = "inject"
	injectStatusAnnotation = "status"
	injectedStatus         = "injected"
	driverImageAnnotation  = "driver-image"
	driverPortAnnotation   = "driver-port"
)

const (
	defaultDriverImageEnv = "DEFAULT_DRIVER_IMAGE"
	defaultDriverPortEnv  = "DEFAULT_DRIVER_PORT"
)

// NewAnnotations returns new annotations helper for the given driver with the given annotations
func NewAnnotations(driver string, annotations map[string]string) DriverAnnotations {
	return DriverAnnotations{
		Name:        driver,
		Annotations: annotations,
	}
}

// DriverAnnotations provides utility functions for driver annotations
type DriverAnnotations struct {
	Name        string
	Annotations map[string]string
}

func (d DriverAnnotations) getAnnotationName(name string) string {
	return fmt.Sprintf(driverAnnotationFormat, d.Name, name)
}

func (d DriverAnnotations) getAnnotation(name string) string {
	return d.Annotations[d.getAnnotationName(name)]
}

func (d DriverAnnotations) setAnnotation(name, value string) {
	d.Annotations[d.getAnnotationName(name)] = value
}

func (d DriverAnnotations) GetInject() (bool, error) {
	inject := d.getAnnotation(injectAnnotation)
	if inject == "" {
		return false, nil
	}
	return strconv.ParseBool(inject)
}

func (d DriverAnnotations) GetInjected() (bool, error) {
	injected := d.getAnnotation(injectStatusAnnotation)
	if injected == "" {
		return false, nil
	} else if injected != injectedStatus {
		return false, fmt.Errorf("unknown inject status %s", injected)
	}
	return true, nil
}

func (d DriverAnnotations) SetInjected(injected bool) {
	if injected {
		d.setAnnotation(injectStatusAnnotation, injectedStatus)
	}
}

func (d DriverAnnotations) GetDriverImage() *string {
	image := d.getAnnotation(driverImageAnnotation)
	if image != "" {
		return &image
	}
	image = os.Getenv(defaultDriverImageEnv)
	if image != "" {
		return &image
	}
	return nil
}

func (d DriverAnnotations) SetDriverImage(image string) {
	d.setAnnotation(driverImageAnnotation, image)
}

func (d DriverAnnotations) GetDriverPort() (*int, error) {
	p := d.getAnnotation(driverPortAnnotation)
	if p != "" {
		i, err := strconv.Atoi(p)
		if err != nil {
			return nil, err
		}
		return &i, nil
	}
	p = os.Getenv(defaultDriverPortEnv)
	if p != "" {
		i, err := strconv.Atoi(p)
		if err != nil {
			return nil, err
		}
		return &i, nil
	}
	return nil, nil
}

func (d DriverAnnotations) SetDriverPort(port int) {
	d.setAnnotation(driverPortAnnotation, strconv.Itoa(port))
}
