package maxmind

import (
	"encoding/json"
	"fmt"
	"log"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
	"github.com/v2fly/geoip/lib"
	"go4.org/netipx"
)

const (
	typeMaxmindMMDBOut = "maxmindMMDB"
	descMaxmindMMDBOut = "Convert data to MaxMind mmdb database format"
)

var (
	defaultMMDBOutputName = "Country.mmdb"
	defaultMMDBOutputDir  = filepath.Join("./", "output", "mmdb")
)

func init() {
	lib.RegisterOutputConfigCreator(typeMaxmindMMDBOut, func(action lib.Action, data json.RawMessage) (lib.OutputConverter, error) {
		return newMaxmindMMDBOut(action, data)
	})
	lib.RegisterOutputConverter(typeMaxmindMMDBOut, &maxmindMMDBOut{
		Description: descMaxmindMMDBOut,
	})
}

func newMaxmindMMDBOut(action lib.Action, data json.RawMessage) (lib.OutputConverter, error) {
	var tmp struct {
		OutputName     string     `json:"outputName"`
		OutputDir      string     `json:"outputDir"`
		Want           []string   `json:"wantedList"`
		Exclude        []string   `json:"excludedList"`
		OneFilePerList bool       `json:"oneFilePerList"`
		OnlyIPType     lib.IPType `json:"onlyIPType"`
	}

	if len(data) > 0 {
		if err := json.Unmarshal(data, &tmp); err != nil {
			return nil, err
		}
	}

	if tmp.OutputName == "" {
		tmp.OutputName = defaultMMDBOutputName
	}

	if tmp.OutputDir == "" {
		tmp.OutputDir = defaultMMDBOutputDir
	}

	return &maxmindMMDBOut{
		Type:           typeMaxmindMMDBOut,
		Action:         action,
		Description:    descMaxmindMMDBOut,
		OutputName:     tmp.OutputName,
		OutputDir:      tmp.OutputDir,
		Want:           tmp.Want,
		Exclude:        tmp.Exclude,
		OneFilePerList: tmp.OneFilePerList,
		OnlyIPType:     tmp.OnlyIPType,
	}, nil
}

type maxmindMMDBOut struct {
	Type           string
	Action         lib.Action
	Description    string
	OutputName     string
	OutputDir      string
	Want           []string
	Exclude        []string
	OneFilePerList bool
	OnlyIPType     lib.IPType
}

func (m *maxmindMMDBOut) GetType() string {
	return m.Type
}

func (m *maxmindMMDBOut) GetAction() lib.Action {
	return m.Action
}

func (m *maxmindMMDBOut) GetDescription() string {
	return m.Description
}

func (m *maxmindMMDBOut) Output(container lib.Container) error {
	// Create output directory
	if err := os.MkdirAll(m.OutputDir, 0755); err != nil {
		return err
	}

	// Get filtered list
	list := m.filterAndSortList(container)

	if m.OneFilePerList {
		// Generate one MMDB file per country/list
		return m.outputOneFilePerList(container, list)
	}

	// Generate single MMDB file with all countries
	return m.outputSingleFile(container, list)
}

func (m *maxmindMMDBOut) outputSingleFile(container lib.Container, list []string) error {
	// Create MMDB writer with appropriate IP version
	writer, err := m.createWriter()
	if err != nil {
		return err
	}

	// Add all entries to the writer
	for _, name := range list {
		entry, found := container.GetEntry(name)
		if !found {
			log.Printf("❌ entry %s not found\n", name)
			continue
		}

		if err := m.addEntryToWriter(writer, entry, name); err != nil {
			return fmt.Errorf("failed to add entry %s: %w", name, err)
		}
	}

	// Write to file
	outputPath := filepath.Join(m.OutputDir, m.OutputName)
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := writer.WriteTo(file); err != nil {
		return err
	}

	log.Printf("✅ [%s] %s --> %s", m.Type, m.OutputName, m.OutputDir)
	return nil
}

func (m *maxmindMMDBOut) outputOneFilePerList(container lib.Container, list []string) error {
	for _, name := range list {
		entry, found := container.GetEntry(name)
		if !found {
			log.Printf("❌ entry %s not found\n", name)
			continue
		}

		// Create MMDB writer for this entry
		writer, err := m.createWriter()
		if err != nil {
			return err
		}

		// Add this entry to the writer
		if err := m.addEntryToWriter(writer, entry, name); err != nil {
			return fmt.Errorf("failed to add entry %s: %w", name, err)
		}

		// Write to file
		filename := strings.ToLower(name) + ".mmdb"
		outputPath := filepath.Join(m.OutputDir, filename)
		file, err := os.Create(outputPath)
		if err != nil {
			return err
		}

		if _, err := writer.WriteTo(file); err != nil {
			file.Close()
			return err
		}
		file.Close()

		log.Printf("✅ [%s] %s --> %s", m.Type, filename, m.OutputDir)
	}

	return nil
}

func (m *maxmindMMDBOut) createWriter() (*mmdbwriter.Tree, error) {
	// Determine IP version for the writer
	ipVersion := 6 // Default to dual-stack (IPv6)
	if m.OnlyIPType == lib.IPv4 {
		ipVersion = 4
	}

	// Create writer with Country database type
	opts := mmdbwriter.Options{
		DatabaseType: "GeoIP2-Country",
		Description: map[string]string{
			"en": "GeoIP2 Country database converted by geoip tool",
		},
		IPVersion:               ipVersion,
		RecordSize:              28,
		IncludeReservedNetworks: true,
	}

	writer, err := mmdbwriter.New(opts)
	if err != nil {
		return nil, err
	}

	return writer, nil
}

func (m *maxmindMMDBOut) addEntryToWriter(writer *mmdbwriter.Tree, entry *lib.Entry, countryCode string) error {
	// Get IP prefixes based on IP type filter
	var prefixes []netip.Prefix

	switch m.OnlyIPType {
	case lib.IPv4:
		ipv4Set, err := entry.GetIPv4Set()
		if err != nil {
			return err
		}
		prefixes = ipv4Set.Prefixes()

	case lib.IPv6:
		ipv6Set, err := entry.GetIPv6Set()
		if err != nil {
			return err
		}
		prefixes = ipv6Set.Prefixes()

	default:
		// Get both IPv4 and IPv6
		ipv4Set, err := entry.GetIPv4Set()
		if err == nil {
			prefixes = append(prefixes, ipv4Set.Prefixes()...)
		}

		ipv6Set, err := entry.GetIPv6Set()
		if err == nil {
			prefixes = append(prefixes, ipv6Set.Prefixes()...)
		}
	}

	// Create country record matching MaxMind GeoIP2 format
	countryRecord := mmdbtype.Map{
		"country": mmdbtype.Map{
			"iso_code": mmdbtype.String(countryCode),
		},
		"registered_country": mmdbtype.Map{
			"iso_code": mmdbtype.String(countryCode),
		},
	}

	// Insert all prefixes into the tree
	for _, prefix := range prefixes {
		ipNet := netipx.PrefixIPNet(prefix)
		if err := writer.Insert(ipNet, countryRecord); err != nil {
			return err
		}
	}

	return nil
}

func (m *maxmindMMDBOut) filterAndSortList(container lib.Container) []string {
	excludeMap := make(map[string]bool)
	for _, exclude := range m.Exclude {
		if exclude = strings.ToUpper(strings.TrimSpace(exclude)); exclude != "" {
			excludeMap[exclude] = true
		}
	}

	wantList := make([]string, 0, len(m.Want))
	for _, want := range m.Want {
		if want = strings.ToUpper(strings.TrimSpace(want)); want != "" && !excludeMap[want] {
			wantList = append(wantList, want)
		}
	}

	if len(wantList) > 0 {
		// Sort the list
		slices.Sort(wantList)
		return wantList
	}

	list := make([]string, 0, 300)
	for entry := range container.Loop() {
		name := entry.GetName()
		if excludeMap[name] {
			continue
		}
		list = append(list, name)
	}

	// Sort the list
	slices.Sort(list)

	return list
}
