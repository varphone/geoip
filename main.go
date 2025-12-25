package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/v2fly/geoip/lib"
)

var (
	list       = flag.Bool("l", false, "List all available input and output formats")
	configFile = flag.String("c", "config.json", "Path to the config file")

	// Quick conversion flags
	input      = flag.String("input", "", "Input format (e.g., maxmindMMDB, text, v2rayGeoIPDat)")
	inputFile  = flag.String("inputFile", "", "Input file path")
	inputName  = flag.String("inputName", "", "Name for the input entry (used for text format)")
	output     = flag.String("output", "", "Output format (e.g., maxmindMMDB, text, v2rayGeoIPDat)")
	outputFile = flag.String("outputFile", "", "Output file path (or directory)")
	wantList   = flag.String("wantList", "", "Comma separated list of wanted countries (e.g., CN,US,JP)")
	onlyIPType = flag.String("onlyIPType", "", "Only process specific IP type: ipv4 or ipv6")
)

func main() {
	flag.Parse()

	if *list {
		lib.ListInputConverter()
		fmt.Println()
		lib.ListOutputConverter()
		return
	}

	// Check if using quick conversion mode
	if *input != "" || *output != "" {
		if err := runQuickConversion(); err != nil {
			log.Fatal(err)
		}
		return
	}

	// Use config file mode
	instance, err := lib.NewInstance()
	if err != nil {
		log.Fatal(err)
	}

	if err := instance.InitConfig(*configFile); err != nil {
		log.Fatal(err)
	}

	if err := instance.Run(); err != nil {
		log.Fatal(err)
	}
}

func runQuickConversion() error {
	if *input == "" {
		return fmt.Errorf("input format is required (use -input)")
	}
	if *inputFile == "" {
		return fmt.Errorf("input file is required (use -inputFile)")
	}
	if *output == "" {
		return fmt.Errorf("output format is required (use -output)")
	}

	instance, err := lib.NewInstance()
	if err != nil {
		return err
	}

	// Build input config
	inputArgs := make(map[string]interface{})
	
	// Handle different input formats
	switch strings.ToLower(*input) {
	case "text":
		// Text format needs either name+uri or inputDir
		if *inputName != "" {
			inputArgs["name"] = *inputName
			inputArgs["uri"] = *inputFile
		} else {
			// If no name is provided, try to extract from filename
			inputArgs["name"] = strings.TrimSuffix(filepath.Base(*inputFile), filepath.Ext(*inputFile))
			inputArgs["uri"] = *inputFile
		}
	default:
		// For other formats (maxmindMMDB, dbipCountryMMDB, etc.)
		inputArgs["uri"] = *inputFile
	}

	if *onlyIPType != "" {
		inputArgs["onlyIPType"] = *onlyIPType
	}

	if *wantList != "" {
		want := strings.Split(*wantList, ",")
		inputArgs["wantedList"] = want
	}

	inputConfig := map[string]interface{}{
		"type":   *input,
		"action": "add",
		"args":   inputArgs,
	}

	// Build output config
	outputArgs := make(map[string]interface{})
	if *outputFile != "" {
		// Determine if outputFile is a file or directory
		if strings.HasSuffix(*outputFile, ".dat") || 
		   strings.HasSuffix(*outputFile, ".mmdb") || 
		   strings.HasSuffix(*outputFile, ".txt") {
			outputArgs["outputName"] = *outputFile
		} else {
			outputArgs["outputDir"] = *outputFile
		}
	}

	if *onlyIPType != "" {
		outputArgs["onlyIPType"] = *onlyIPType
	}

	if *wantList != "" {
		want := strings.Split(*wantList, ",")
		outputArgs["wantedList"] = want
	}

	outputConfig := map[string]interface{}{
		"type":   *output,
		"action": "output",
		"args":   outputArgs,
	}

	// Build complete config
	config := map[string]interface{}{
		"input":  []interface{}{inputConfig},
		"output": []interface{}{outputConfig},
	}

	configBytes, err := json.Marshal(config)
	if err != nil {
		return err
	}

	// Initialize and run
	if err := instance.InitConfigFromBytes(configBytes); err != nil {
		return err
	}

	fmt.Printf("Converting from %s (%s) to %s format...\n", *input, *inputFile, *output)
	if err := instance.Run(); err != nil {
		return err
	}

	fmt.Println("Conversion completed successfully!")
	return nil
}
