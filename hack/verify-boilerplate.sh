#!/bin/bash

# Copyright 2014 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

KUBE_ROOT=$(dirname "${BASH_SOURCE}")/..

boilerDir="${KUBE_ROOT}/hack/boilerplate"
boiler="${boilerDir}/boilerplate.py"

all_files=$(find . -name '*.go' | grep -v "./vendor/")

excluded_files=("./pkg/proxy/namespace_round_tripper.go" "./pkg/proxy/namespace_round_tripper_test.go")

files_to_check=()
for file in $all_files; do
  should_exclude=false
  for excluded_file in "${excluded_files[@]}"; do
    if [[ "$file" == "$excluded_file" ]]; then
      should_exclude=true
      break
    fi
  done
  if ! $should_exclude; then
    files_to_check+=("$file")
  fi
done

files_need_boilerplate=($(python3 ${boiler} "${files_to_check[@]}"))

# Run boilerplate check
if [[ ${#files_need_boilerplate[@]} -gt 0 ]]; then
  for file in "${files_need_boilerplate[@]}"; do
    echo "Boilerplate header is wrong for: ${file}"
  done

  exit 1
fi
