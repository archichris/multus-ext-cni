// Copyright 2015 CNI authors
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

package disk

import (
	"bufio"
	"io"
	"io/ioutil"

	// "log"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/containernetworking/plugins/plugins/ipam/host-local/backend"
	"github.com/intel/multus-cni/logging"
	"github.com/intel/multus-cni/multus-ipam/backend/allocator"
)

const lastIPFilePrefix = "last_reserved_ip."
const LineBreak = "\r\n"

var defaultDataDir = "/var/lib/cni/networks"
var cacheName = "rangeset_cache"

// Store is a simple disk-backed store that creates one file per IP
// address in a given directory. The contents of the file are the container ID.
type Store struct {
	*FileLock
	dataDir string
}

// Store implements the Store interface
var _ backend.Store = &Store{}

func New(network, dataDir string) (*Store, error) {
	if dataDir == "" {
		dataDir = defaultDataDir
	}
	dir := filepath.Join(dataDir, network)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	lk, err := NewFileLock(dir)
	if err != nil {
		return nil, err
	}
	return &Store{lk, dir}, nil
}

func (s *Store) Reserve(id string, ifname string, ip net.IP, rangeID string) (bool, error) {
	fname := GetEscapedPath(s.dataDir, ip.String())

	f, err := os.OpenFile(fname, os.O_RDWR|os.O_EXCL|os.O_CREATE, 0644)
	if os.IsExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := f.WriteString(strings.TrimSpace(id) + LineBreak + ifname); err != nil {
		f.Close()
		os.Remove(f.Name())
		return false, err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return false, err
	}
	// store the reserved ip in lastIPFile
	ipfile := GetEscapedPath(s.dataDir, lastIPFilePrefix+rangeID)
	err = ioutil.WriteFile(ipfile, []byte(ip.String()), 0644)
	if err != nil {
		return false, err
	}
	return true, nil
}

// LastReservedIP returns the last reserved IP if exists
func (s *Store) LastReservedIP(rangeID string) (net.IP, error) {
	ipfile := GetEscapedPath(s.dataDir, lastIPFilePrefix+rangeID)
	data, err := ioutil.ReadFile(ipfile)
	if err != nil {
		return nil, err
	}
	return net.ParseIP(string(data)), nil
}

func (s *Store) Release(ip net.IP) error {
	return os.Remove(GetEscapedPath(s.dataDir, ip.String()))
}

func (s *Store) FindByKey(id string, ifname string, match string) (bool, error) {
	found := false

	err := filepath.Walk(s.dataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		data, err := ioutil.ReadFile(path)
		if err != nil {
			return nil
		}
		if strings.TrimSpace(string(data)) == match {
			found = true
		}
		return nil
	})
	return found, err

}

func (s *Store) FindByID(id string, ifname string) bool {
	s.Lock()
	defer s.Unlock()

	found := false
	match := strings.TrimSpace(id) + LineBreak + ifname
	found, err := s.FindByKey(id, ifname, match)

	// Match anything created by this id
	if !found && err == nil {
		match := strings.TrimSpace(id)
		found, err = s.FindByKey(id, ifname, match)
	}

	return found
}

func (s *Store) ReleaseByKey(id string, ifname string, match string) (bool, error) {
	found := false
	err := filepath.Walk(s.dataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		data, err := ioutil.ReadFile(path)
		if err != nil {
			return nil
		}
		if strings.TrimSpace(string(data)) == match {
			if err := os.Remove(path); err != nil {
				return nil
			}
			found = true
		}
		return nil
	})
	return found, err

}

// N.B. This function eats errors to be tolerant and
// release as much as possible
func (s *Store) ReleaseByID(id string, ifname string) error {
	found := false
	match := strings.TrimSpace(id) + LineBreak + ifname
	found, err := s.ReleaseByKey(id, ifname, match)

	// For backwards compatibility, look for files written by a previous version
	if !found && err == nil {
		match := strings.TrimSpace(id)
		found, err = s.ReleaseByKey(id, ifname, match)
	}
	return err
}

// GetByID returns the IPs which have been allocated to the specific ID
func (s *Store) GetByID(id string, ifname string) []net.IP {
	var ips []net.IP

	match := strings.TrimSpace(id) + LineBreak + ifname
	// matchOld for backwards compatibility
	matchOld := strings.TrimSpace(id)

	// walk through all ips in this network to get the ones which belong to a specific ID
	_ = filepath.Walk(s.dataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		data, err := ioutil.ReadFile(path)
		if err != nil {
			return nil
		}
		if strings.TrimSpace(string(data)) == match || strings.TrimSpace(string(data)) == matchOld {
			_, ipString := filepath.Split(path)
			if ip := net.ParseIP(ipString); ip != nil {
				ips = append(ips, ip)
			}
		}
		return nil
	})

	return ips
}

func GetEscapedPath(dataDir string, fname string) string {
	if runtime.GOOS == "windows" {
		fname = strings.Replace(fname, ":", "_", -1)
	}
	return filepath.Join(dataDir, fname)
}

func (s *Store) GetDir() string {
	return s.dataDir
}

// LoadRangeSetFromCache is used to load IP range set "startIP:endIP" from cache file
func (s *Store) LoadCache() ([]allocator.SimpleRange, error) {
	s.Lock()
	defer s.Unlock()
	fname := GetEscapedPath(s.dataDir, cacheName)
	result := []allocator.SimpleRange{}
	_, err := os.Stat(fname)
	if os.IsNotExist(err) { // file do not exist
		return result, nil
	}
	f, err := os.Open(fname)
	if os.IsExist(err) {
		return nil, err
	}
	defer f.Close()
	buf := bufio.NewReader(f)
	for {
		line, err := buf.ReadString('\n')
		if err != nil {
			if err == io.EOF { //读取结束，会报EOF
				return result, nil
			}
			return nil, err
		}
		line = strings.TrimRight(line, "\n\r\t ")
		pairIP := strings.Split(line, "-")
		// logging.Debugf("load cache %v", pairIP)
		result = append(result, allocator.SimpleRange{net.ParseIP(pairIP[0]), net.ParseIP(pairIP[1])})
	}
}

func (s *Store) FlashCache(srs []allocator.SimpleRange) error {
	logging.Debugf("Going to flash cache %v", srs)
	s.Lock()
	defer s.Unlock()
	fname := GetEscapedPath(s.dataDir, cacheName)
	f, err := os.OpenFile(fname, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return logging.Errorf("open file %v failed, %v", fname, err)
	}
	defer f.Close()
	defer f.Sync()
	for _, sr := range srs {
		sr.Canonicalize()
		_, err = f.WriteString(fmt.Sprintf("%s-%s\n", sr.RangeStart.String(), sr.RangeEnd.String()))
		if err != nil {
			return logging.Errorf("write file %s failed, %v", fname, err)
		}
	}
	return nil
}

func (s *Store) AppendCache(sr *allocator.SimpleRange) error {
	logging.Debugf("Going to append cache %v", *sr)
	caches, err := s.LoadCache()
	if err != nil {
		return err
	}

	for _, csr := range caches {
		if csr.Overlaps(sr) {
			return logging.Errorf("%v over laps cache %v", *sr, csr)
		}
	}
	caches = append(caches, *sr)
	return s.FlashCache(caches)
}

func (s *Store) DeleteCache(sr *allocator.SimpleRange) error {
	caches, err := s.LoadCache()
	if err != nil {
		return err
	}
	for idx, cache := range caches {
		if cache.Overlaps(sr) {
			if idx == 0 {
				caches = caches[1:]
			} else if idx == len(caches)-1 {
				caches = caches[:idx]
			} else {
				caches = append(caches[:idx], caches[idx+1:]...)
			}
			break
		}
	}
	return s.FlashCache(caches)
}

func GetAllNet(d string) []string {
	dir := d
	if dir == "" {
		dir = defaultDataDir
	}

	networks := []string{}

	logging.Debugf("data dir is %v", dir)

	files, _ := ioutil.ReadDir(dir)
	for _, file := range files {
		if file.IsDir() {
			cacheFile := filepath.Join(dir, file.Name(), cacheName)
			_, err := os.Stat(cacheFile)
			if err == nil {
				networks = append(networks, file.Name())
			}
		}
	}
	return networks
}
