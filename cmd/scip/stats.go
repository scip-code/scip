package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"

	"errors"

	"github.com/hhatto/gocloc"
	"github.com/montanaflynn/stats"
	"github.com/urfave/cli/v3"
	"google.golang.org/protobuf/proto"

	"github.com/scip-code/scip/bindings/go/scip"
)

type statsFlags struct {
	from              string
	customProjectRoot string
}

func statsCommand() cli.Command {
	var statsFlags statsFlags
	stats := cli.Command{
		Name:  "stats",
		Usage: "Output useful statistics about a SCIP index",
		Flags: []cli.Flag{
			fromFlag(&statsFlags.from),
			projectRootFlag(&statsFlags.customProjectRoot),
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return statsMain(statsFlags)
		},
	}
	return stats
}

func statsMain(flags statsFlags) error {
	from := flags.from
	index, err := readFromOption(from)
	if err != nil {
		return fmt.Errorf("error reading SCIP file: %w", err)
	}
	if index.Metadata == nil {
		return fmt.Errorf("Index.Metadata is nil (--from=%s)", from)
	}
	output := map[string]interface{}{}
	indexStats, err := countStatistics(index, flags.customProjectRoot)
	if err != nil {
		return fmt.Errorf("error counting stats: %w", err)
	}
	jsonBytes, err := json.MarshalIndent(indexStats, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshall into JSON %s: %w", output, err)
	}
	fmt.Println(string(jsonBytes))
	return nil
}

type Stats struct {
	Percentiles struct {
		Fifty               int32 `json:"50"`
		Ninety              int32 `json:"90"`
		NinetyFive          int32 `json:"95"`
		NinetyNine          int32 `json:"99"`
		NinetyNinePointNine int32 `json:"99.9"`
	}
	Mean    int32  `json:"mean"`
	Stddev  int32  `json:"stddev"`
	Max     int32  `json:"max"`
	Sum     int32  `json:"sum"`
	Comment string `json:"comment"`
}

func NewStats(values []float64) Stats {
	s := Stats{}
	s.Percentiles.Fifty = percentile(values, 50)
	s.Percentiles.Ninety = percentile(values, 90)
	s.Percentiles.NinetyFive = percentile(values, 95)
	s.Percentiles.NinetyNine = percentile(values, 99)
	s.Percentiles.NinetyNinePointNine = percentile(values, 99.9)
	mean, _ := stats.Mean(values)
	s.Mean = int32(mean)
	stddev, _ := stats.StandardDeviation(values)
	s.Stddev = int32(stddev)
	max, _ := stats.Max(values)
	s.Max = int32(max)
	sum, _ := stats.Sum(values)
	s.Sum = int32(sum)
	return s
}

type indexStatistics struct {
	Documents        int32 `json:"documents"`
	DocumentSizes    Stats `json:"documentSizes"`
	LinesOfCode      int32 `json:"linesOfCode"`
	Occurrences      int32 `json:"occurrences"`
	OccurrenceCounts Stats `json:"occurrenceCounts"`
	Definitions      int32 `json:"definitions"`
	DefinitionCounts Stats `json:"definitionCounts"`
}

func countStatistics(index *scip.Index, customProjectRoot string) (*indexStatistics, error) {
	loc, err := countLinesOfCode(index, customProjectRoot)
	var linesOfCode int32
	if err != nil {
		// Keep this a non-fatal error so that we can measure other index stats
		// even if the project is not cloned locally (e.g. if it's a huge
		// project like Chromium or the Linux kernel).
		log.Printf("Couldn't count lines of code: %s", err)
	} else {
		linesOfCode = loc.Total.Code
	}
	stats := &indexStatistics{
		Documents:   int32(len(index.Documents)),
		LinesOfCode: linesOfCode,
		Occurrences: 0,
		Definitions: 0,
	}
	documentSizes := []float64{}
	occurrenceCounts := []float64{}
	definitionCounts := []float64{}
	for _, document := range index.Documents {
		bytes, _ := proto.Marshal(document)
		documentSizes = append(documentSizes, float64(len(bytes)))
		stats.Occurrences += int32(len(document.Occurrences))
		occurrenceCounts = append(occurrenceCounts, float64(len(document.Occurrences)))
		definitionCounts = append(definitionCounts, 0)
		for _, occurrence := range document.Occurrences {
			if scip.SymbolRole_Definition.Matches(occurrence) {
				stats.Definitions += 1
				definitionCounts[len(definitionCounts)-1] += 1
			}
		}
	}
	stats.DocumentSizes = NewStats(documentSizes)
	stats.DocumentSizes.Comment = "sizes are in bytes"
	stats.OccurrenceCounts = NewStats(occurrenceCounts)
	stats.DefinitionCounts = NewStats(definitionCounts)
	stats.DefinitionCounts.Comment = "counted using occurrences"
	return stats, nil
}

func percentile(buf []float64, percent float64) int32 {
	res, _ := stats.Percentile(buf, percent)
	return int32(res)
}

func countLinesOfCode(index *scip.Index, customProjectRoot string) (*gocloc.Result, error) {
	var localSource string
	root, err := url.Parse(index.Metadata.ProjectRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Index.Metadata.ProjectRoot as a URI %s: %w", index.Metadata.ProjectRoot, err)
	}
	if customProjectRoot != "" {
		localSource = customProjectRoot
	} else {
		_, err := os.Stat(root.Path)
		if errors.Is(err, os.ErrNotExist) {
			cwd, _ := os.Getwd()
			log.Printf("Project root [%s] doesn't exist, using current working directory [%s] instead. "+
				"To override this behaviour, specify --project-root=<folder> option", root.Path, cwd)
			localSource = cwd
		} else if err != nil {
			return nil, err
		}
	}

	stat, err := os.Stat(localSource)
	if err != nil {
		return nil, err
	}
	if !stat.IsDir() {
		return nil, fmt.Errorf("Project root [%s] is not a directory", localSource)
	}
	processor := gocloc.NewProcessor(gocloc.NewDefinedLanguages(), gocloc.NewClocOptions())
	var paths []string
	for _, document := range index.Documents {
		paths = append(paths, filepath.Join(localSource, document.RelativePath))
	}
	return processor.Analyze(paths)
}
