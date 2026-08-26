package steamstats

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/jslay88/vdf"
	"github.com/seb/omarchy-plugin-steam/internal/vdfbin"
)

const (
	magicPackageV27 = 0x06565527
	magicPackageV28 = 0x06565528
	packageEnd      = 0xFFFFFFFF
)

var (
	libraryPathRE = regexp.MustCompile(`"path"\s+"([^"]+)"`)
	appIDRE       = regexp.MustCompile(`"appid"\s+"(\d+)"`)
	nameRE        = regexp.MustCompile(`"name"\s+"([^"]+)"`)
)

var skipNameTokens = []string{
	"proton",
	"steamworks",
	"steam linux runtime",
	"redistributable",
}

type Result struct {
	OK               bool   `json:"ok"`
	Error            string `json:"error,omitempty"`
	InstalledCount   int    `json:"installed_count,omitempty"`
	OwnedCount       int    `json:"owned_count,omitempty"`
	TotalPlaytimeMin int    `json:"total_playtime_min,omitempty"`
}

func Compute(steamDir, localconfigPath string) (Result, error) {
	info, err := os.Stat(steamDir)
	if err != nil || !info.IsDir() {
		return Result{OK: false, Error: "Steam directory not found"}, nil
	}

	installed, err := installedCount(steamDir)
	if err != nil {
		return Result{OK: false, Error: err.Error()}, nil
	}
	owned, err := ownedCount(steamDir)
	if err != nil {
		return Result{OK: false, Error: err.Error()}, nil
	}
	playtime, err := totalPlaytimeMin(localconfigPath)
	if err != nil {
		return Result{OK: false, Error: err.Error()}, nil
	}

	return Result{
		OK:               true,
		InstalledCount:   installed,
		OwnedCount:       owned,
		TotalPlaytimeMin: playtime,
	}, nil
}

func libraryPaths(steamDir string) ([]string, error) {
	paths := []string{}
	seen := map[string]struct{}{}

	steamapps := filepath.Join(steamDir, "steamapps")
	if st, err := os.Stat(steamapps); err == nil && st.IsDir() {
		paths = append(paths, steamapps)
		if abs, err := filepath.EvalSymlinks(steamapps); err == nil {
			seen[abs] = struct{}{}
		}
	}

	vdfPath := filepath.Join(steamapps, "libraryfolders.vdf")
	data, err := os.ReadFile(vdfPath)
	if err != nil {
		return paths, nil
	}

	for _, match := range libraryPathRE.FindAllStringSubmatch(string(data), -1) {
		lib := filepath.Join(strings.ReplaceAll(match[1], `\\`, `\`), "steamapps")
		st, err := os.Stat(lib)
		if err != nil || !st.IsDir() {
			continue
		}
		abs, err := filepath.EvalSymlinks(lib)
		if err != nil {
			abs = lib
		}
		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}
		paths = append(paths, lib)
	}
	return paths, nil
}

func installedCount(steamDir string) (int, error) {
	libs, err := libraryPaths(steamDir)
	if err != nil {
		return 0, err
	}

	seen := map[string]struct{}{}
	count := 0
	for _, lib := range libs {
		matches, err := filepath.Glob(filepath.Join(lib, "appmanifest_*.acf"))
		if err != nil {
			return 0, err
		}
		for _, manifest := range matches {
			data, err := os.ReadFile(manifest)
			if err != nil {
				continue
			}
			text := string(data)
			appMatch := appIDRE.FindStringSubmatch(text)
			if len(appMatch) < 2 {
				continue
			}
			appid := appMatch[1]
			if _, ok := seen[appid]; ok {
				continue
			}
			seen[appid] = struct{}{}
			name := ""
			if nameMatch := nameRE.FindStringSubmatch(text); len(nameMatch) >= 2 {
				name = strings.ToLower(nameMatch[1])
			}
			if isToolingName(name) {
				continue
			}
			count++
		}
	}
	return count, nil
}

func isToolingName(name string) bool {
	for _, token := range skipNameTokens {
		if strings.Contains(name, token) {
			return true
		}
	}
	return false
}

func ownedCount(steamDir string) (int, error) {
	owned, err := ownedAppIDs(steamDir)
	if err != nil {
		return 0, err
	}
	if len(owned) == 0 {
		return 0, nil
	}
	types, err := appInfoTypeMap(steamDir)
	if err != nil {
		return 0, err
	}
	if len(types) == 0 {
		return 0, nil
	}
	count := 0
	for appid := range owned {
		if types[appid] == "game" {
			count++
		}
	}
	return count, nil
}

func ownedAppIDs(steamDir string) (map[uint32]struct{}, error) {
	path := filepath.Join(steamDir, "appcache", "packageinfo.vdf")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(data) < 8 {
		return nil, nil
	}

	magic := binary.LittleEndian.Uint32(data[:4])
	var headerSkip int
	switch magic {
	case magicPackageV27:
		headerSkip = 24
	case magicPackageV28:
		headerSkip = 32
	default:
		return nil, nil
	}

	return ownedAppIDsFromReader(data, headerSkip)
}

func ownedAppIDsFromReader(data []byte, headerSkip int) (map[uint32]struct{}, error) {
	owned := map[uint32]struct{}{}
	offset := 8
	for offset+4 <= len(data) {
		pkgID := binary.LittleEndian.Uint32(data[offset : offset+4])
		offset += 4
		if pkgID == packageEnd {
			break
		}
		if offset+headerSkip > len(data) {
			break
		}
		offset += headerSkip

		block, err := vdfbin.Load(data[offset:], nil)
		if err != nil {
			return nil, err
		}
		consumed, err := measureBinaryVDF(data[offset:])
		if err != nil {
			return nil, err
		}
		offset += consumed

		for _, value := range block {
			pkg, ok := asMap(value)
			if !ok {
				continue
			}
			appids, ok := asMap(pkg["appids"])
			if !ok {
				continue
			}
			for _, raw := range appids {
				appid, ok := asUint(raw)
				if !ok {
					continue
				}
				owned[appid] = struct{}{}
			}
		}
	}
	return owned, nil
}

func measureBinaryVDF(data []byte) (int, error) {
	pos := 0
	depth := 0
	for pos < len(data) {
		t := data[pos]
		pos++
		if t == typeEndVDF {
			if depth == 0 {
				return pos, nil
			}
			depth--
			continue
		}

		// key: null-terminated string
		start := pos
		for pos < len(data) && data[pos] != 0 {
			pos++
		}
		if pos >= len(data) {
			return 0, fmt.Errorf("unterminated key")
		}
		pos++
		_ = start

		switch t {
		case 0x00:
			depth++
		case 0x01, 0x05:
			for pos < len(data) {
				if t == 0x05 {
					if pos+1 < len(data) && data[pos] == 0 && data[pos+1] == 0 {
						pos += 2
						break
					}
					pos += 2
					continue
				}
				if data[pos] == 0 {
					pos++
					break
				}
				pos++
			}
		case 0x02, 0x03, 0x04, 0x06:
			pos += 4
		case 0x07, 0x0A:
			pos += 8
		default:
			return 0, fmt.Errorf("unknown type 0x%02x", t)
		}
	}
	return 0, fmt.Errorf("binary vdf not terminated")
}

const typeEndVDF = 0x08

func appInfoTypeMap(steamDir string) (map[uint32]string, error) {
	path := filepath.Join(steamDir, "appcache", "appinfo.vdf")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(data) < 8 {
		return nil, nil
	}

	magic := string(data[:4])
	switch magic {
	case "(DV\x07":
		return parseAppInfoV40(data)
	case ")DV\x07":
		return parseAppInfoV41(data)
	default:
		return nil, nil
	}
}

func parseAppInfoV40(data []byte) (map[uint32]string, error) {
	types := map[uint32]string{}
	sectionSize := 68
	i := 8
	for i+sectionSize <= len(data)-4 {
		appid := binary.LittleEndian.Uint32(data[i : i+4])
		entrySize := binary.LittleEndian.Uint32(data[i+4 : i+8])
		vdfSize := int(entrySize) - (sectionSize - 8)
		i += sectionSize
		if vdfSize <= 0 || i+vdfSize > len(data) {
			break
		}
		block, err := vdfbin.Load(data[i:i+vdfSize], nil)
		if err != nil {
			return nil, err
		}
		i += vdfSize
		if appType := extractAppType(block); appid != 0 && appType != "" {
			types[appid] = appType
		}
	}
	return types, nil
}

func parseAppInfoV41(data []byte) (map[uint32]string, error) {
	types := map[uint32]string{}
	keyTableOffset := int64(binary.LittleEndian.Uint64(data[8:16]))
	keyCount := int(binary.LittleEndian.Uint32(data[keyTableOffset : keyTableOffset+4]))

	keyTable := make([]string, 0, keyCount)
	tableI := int(keyTableOffset) + 4
	for range keyCount {
		start := tableI
		for tableI < len(data) && data[tableI] != 0 {
			tableI++
		}
		if tableI >= len(data) {
			break
		}
		keyTable = append(keyTable, string(data[start:tableI]))
		tableI++
	}

	sectionSize := 68
	i := 16
	for i+sectionSize <= int(keyTableOffset)-4 {
		appid := binary.LittleEndian.Uint32(data[i : i+4])
		entrySize := binary.LittleEndian.Uint32(data[i+4 : i+8])
		vdfSize := int(entrySize) - (sectionSize - 8)
		i += sectionSize
		if vdfSize <= 0 || i+vdfSize > len(data) {
			break
		}
		block, err := vdfbin.Load(data[i:i+vdfSize], keyTable)
		if err != nil {
			return nil, err
		}
		i += vdfSize
		if appType := extractAppType(block); appid != 0 && appType != "" {
			types[appid] = appType
		}
	}
	return types, nil
}

func extractAppType(block vdfbin.Map) string {
	for _, value := range block {
		child, ok := asMap(value)
		if !ok {
			continue
		}
		common, ok := asMap(child["common"])
		if !ok {
			continue
		}
		if raw, ok := common["type"]; ok {
			return strings.ToLower(fmt.Sprint(raw))
		}
	}
	return ""
}

func totalPlaytimeMin(localconfigPath string) (int, error) {
	if localconfigPath == "" || localconfigPath == "/dev/null" {
		return 0, nil
	}
	doc, err := vdf.ParseFile(localconfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	root := doc.Get("UserLocalConfigStore")
	if root == nil {
		return 0, nil
	}
	software := root.Get("Software")
	if software == nil {
		return 0, nil
	}
	valve := software.Get("Valve")
	if valve == nil {
		return 0, nil
	}
	steam := valve.Get("Steam")
	if steam == nil {
		return 0, nil
	}
	apps := steam.Get("apps")
	if apps == nil || !apps.IsObject {
		return 0, nil
	}

	total := 0
	for _, app := range apps.Children {
		if app.IsObject {
			if pt := app.Get("Playtime"); pt != nil {
				if n, err := strconv.Atoi(pt.Value); err == nil {
					total += n
				}
			}
			continue
		}
	}
	return total, nil
}

func asMap(value any) (vdfbin.Map, bool) {
	switch v := value.(type) {
	case vdfbin.Map:
		return v, true
	case map[string]any:
		out := vdfbin.Map(v)
		return out, true
	case vdf.Map:
		out := vdfbin.Map{}
		for k, raw := range v {
			out[k] = raw
		}
		return out, true
	default:
		return nil, false
	}
}

func asUint(value any) (uint32, bool) {
	switch v := value.(type) {
	case uint32:
		return v, true
	case int:
		if v < 0 {
			return 0, false
		}
		return uint32(v), true
	case int32:
		if v < 0 {
			return 0, false
		}
		return uint32(v), true
	case string:
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return 0, false
		}
		return uint32(n), true
	default:
		return 0, false
	}
}
