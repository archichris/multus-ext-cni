// Copyright 2017 CNI authors
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

package allocator

import (
	"fmt"
	"net"
	"strings"
)

// Contains returns true if any range in this set contains an IP
func (s *RangeSet) Contains(addr net.IP) bool {
	r, _ := s.RangeFor(addr)
	return r != nil
}

// RangeFor finds the range that contains an IP, or nil if not found
func (s *RangeSet) RangeFor(addr net.IP) (*Range, error) {
	if err := canonicalizeIP(&addr); err != nil {
		return nil, err
	}

	for _, r := range *s {
		if r.Contains(addr) {
			return &r, nil
		}
	}

	return nil, fmt.Errorf("%s not in range set %s", addr.String(), s.String())
}

// Overlaps returns true if any ranges in any set overlap with this one
func (s *RangeSet) Overlaps(p1 *RangeSet) bool {
	for _, r := range *s {
		for _, r1 := range *p1 {
			if r.Overlaps(&r1) {
				return true
			}
		}
	}
	return false
}

// Canonicalize ensures the RangeSet is in a standard form, and detects any
// invalid input. Call Range.Canonicalize() on every Range in the set
func (s *RangeSet) Canonicalize() error {
	if len(*s) == 0 {
		return fmt.Errorf("empty range set")
	}

	fam := 0
	for i := range *s {
		if err := (*s)[i].Canonicalize(); err != nil {
			return err
		}
		if i == 0 {
			fam = len((*s)[i].RangeStart)
		} else {
			if fam != len((*s)[i].RangeStart) {
				return fmt.Errorf("mixed address families")
			}
		}
	}

	//Check if the ranges overlap
	l := len(*s)
	for i, r1 := range (*s)[:l-1] {
		for _, r2 := range (*s)[i+1:] {
			if r1.Overlaps(&r2) {
				return fmt.Errorf("ranges %s and %s overlap", r1.String(), r2.String())
			}
			// all the range set in the same group shall have the same subnet
			// Todo apply to the new style
			// if r1.SubnetStr() != r2.SubnetStr() {
			// 	return fmt.Errorf("range %s and %s are in the same group, but have different subnet (%s, %s)",
			// 		r1.String(), r2.String(), r1.SubnetStr(), r2.SubnetStr())
			// }
		}
	}

	return nil
}

func (s *RangeSet) String() string {
	out := []string{}
	for _, r := range *s {
		out = append(out, r.String())
	}

	return strings.Join(out, ",")
}
