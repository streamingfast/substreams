// Copyright 2024 StreamingFast Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sqe

type FieldTransformer interface {
	// TransformFieldName receives the field name and allow receiver of the invocation to update its name. The field's
	// name is updated if the invocation returns a nil error.
	TransformFieldName(field string) (string, error)

	// TransformStringLiteral receives the field name (the updated one from a prior invocation of `TransformFieldName`)
	// and a string literal (either a direct one or a sub-element from a `StringList`) and allows transformation of the
	// `StringLiteral` value in place.
	TransformStringLiteral(field string, value *StringLiteral) error
}

type noOpTransformer struct{}

func (noOpTransformer) TransformFieldName(field string) (string, error) {
	return field, nil
}

func (noOpTransformer) TransformStringLiteral(field string, value *StringLiteral) error {
	return nil
}

var NoOpFieldTransformer noOpTransformer
