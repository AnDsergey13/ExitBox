// ExitBox - Multi-Agent Container Sandbox
// Copyright (C) 2026 Cloud Exit B.V.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package toolbox

import (
	"reflect"
	"testing"
)

func TestParseGoVersions(t *testing.T) {
	data := []byte(`[
		{"version":"go1.27.0","stable":true,"files":[{"os":"linux","arch":"amd64"},{"os":"linux","arch":"arm64"}]},
		{"version":"go1.27rc1","stable":false,"files":[{"os":"linux","arch":"amd64"}]},
		{"version":"go1.26.3","stable":true,"files":[{"os":"linux","arch":"amd64"}]},
		{"version":"go1.26.2","stable":true,"files":[{"os":"darwin","arch":"amd64"}]}
	]`)
	versions, err := parseGoVersions(data)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"1.27.0", "1.26.3"}
	if !reflect.DeepEqual(versions, want) {
		t.Errorf("parseGoVersions = %v, want %v", versions, want)
	}
}

func TestNormalizeVersionList(t *testing.T) {
	in := []string{"1.10.0", "1.9.2", "1.10.0", "go1.10.0", "1.2.0-beta1", ""}
	got := normalizeVersionList(in)
	want := []string{"1.10.0", "1.9.2", "1.2.0-beta1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("normalizeVersionList = %v, want %v", got, want)
	}
}

func TestCompareVersionNumbers(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.27.0", "1.26.3", 1},
		{"1.26.3", "1.27.0", -1},
		{"1.27", "1.27.0", 0},
		{"22.14.0-r0", "22.15.0-r0", -1},
		{"1.2.3", "1.2.3-beta1", 0},
	}
	for _, tt := range tests {
		got := compareVersionNumbers(tt.a, tt.b)
		if (got > 0 && tt.want <= 0) || (got < 0 && tt.want >= 0) || (got == 0 && tt.want != 0) {
			t.Errorf("compareVersionNumbers(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
