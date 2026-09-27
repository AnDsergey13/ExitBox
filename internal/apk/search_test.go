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

package apk

import (
	"reflect"
	"testing"
)

func TestParseIndexIncludesVersions(t *testing.T) {
	data := []byte(`P:nodejs
V:22.14.0-r0
T:Node.js runtime

P:python3
V:3.12.9-r0
T:Python

P:rust
V:1.85.0-r0
T:Rust
`)
	pkgs := parseIndex(data)
	if len(pkgs) != 3 {
		t.Fatalf("parseIndex len = %d, want 3", len(pkgs))
	}
	if pkgs[0].Name != "nodejs" || pkgs[0].Version != "22.14.0-r0" {
		t.Errorf("pkgs[0] = %+v, want nodejs 22.14.0-r0", pkgs[0])
	}
}

func TestMatchVersion(t *testing.T) {
	versions := []string{"3.13.0-r0", "3.12.9-r0", "3.12.8-r0"}

	tests := []struct {
		requested string
		want      string
		wantErr   bool
	}{
		{"3.12", "3.12.9-r0", false},
		{"3.12.9", "3.12.9-r0", false},
		{"3.12.9-r0", "3.12.9-r0", false},
		{"3.13", "3.13.0-r0", false},
		{"4.0", "", true},
	}
	for _, tt := range tests {
		got, err := matchVersion(versions, tt.requested, "python3")
		if tt.wantErr {
			if err == nil {
				t.Errorf("matchVersion(%q) expected error, got %q", tt.requested, got)
			}
			continue
		}
		if err != nil {
			t.Fatalf("matchVersion(%q) error: %v", tt.requested, err)
		}
		if got != tt.want {
			t.Errorf("matchVersion(%q) = %q, want %q", tt.requested, got, tt.want)
		}
	}
}

func TestCompareAlpineVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"3.12.9-r0", "3.12.8-r0", 1},
		{"3.12.8-r0", "3.12.9-r0", -1},
		{"3.12.9-r0", "3.12.9-r0", 0},
		{"3.12.9-r1", "3.12.9-r0", 1},
		{"3.13.0-r0", "3.12.9-r0", 1},
	}
	for _, tt := range tests {
		got := compareAlpineVersions(tt.a, tt.b)
		if (got < 0 && tt.want >= 0) || (got > 0 && tt.want <= 0) || (got == 0 && tt.want != 0) {
			t.Errorf("compareAlpineVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestSortVersionsDesc(t *testing.T) {
	in := []string{"3.12.9-r0", "3.13.0-r0", "3.12.9-r1", "3.12.8-r0"}
	sortVersionsDesc(in)
	want := []string{"3.13.0-r0", "3.12.9-r1", "3.12.9-r0", "3.12.8-r0"}
	if !reflect.DeepEqual(in, want) {
		t.Errorf("sortVersionsDesc = %v, want %v", in, want)
	}
}
