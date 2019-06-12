/*
 * Copyright 2019 Open Networking Foundation
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package v1alpha1

func SetDefaults_Partition(partition *Partition) {
	if partition.Spec.Size == 0 {
		partition.Spec.Size = 1
	}
}

func SetDefaults_PartitionGroup(group *PartitionGroup) {
	if group.Spec.Partitions == 0 {
		group.Spec.Partitions = 1
	}
	if group.Spec.PartitionSize == 0 {
		group.Spec.PartitionSize = 1
	}
}
