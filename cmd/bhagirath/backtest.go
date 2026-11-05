package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vivekyarra/bhagirath/internal/backtest"
	"github.com/vivekyarra/bhagirath/internal/config"
	"github.com/vivekyarra/bhagirath/internal/domain"
)

// runBacktest replays outbreak timelines through the frozen rule.
func runBacktest(args []string) error {
	fs := flag.NewFlagSet("backtest", flag.ContinueOnError)
	dir := fs.String("dir", "backtest", "directory with outbreaks.yaml, sensitivity.yaml and data/*.csv")
	ruleFile := fs.String("rule", "config/alert_rule_v1.yaml", "frozen rule file")
	ladderFile := fs.String("ladder", "config/ladder.example.yaml", "ladder file (deadline hours)")
	out := fs.String("o", "", "write RESULTS.md here (default: <dir>/RESULTS.md); '-' for stdout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	r, err := loadRule(*ruleFile)
	if err != nil {
		return err
	}
	l, err := config.LoadLadder(*ladderFile)
	if err != nil {
		return err
	}
	hours := map[domain.Level]int{}
	for _, lv := range []domain.Level{domain.LevelWatch, domain.LevelWarning, domain.LevelCritical} {
		hours[lv] = l.Deadline(lv)
	}
	obs, vs, err := backtest.LoadDir(*dir)
	if err != nil {
		return err
	}
	results := backtest.Run(r, obs, vs, hours)
	md := backtest.Markdown(r, obs, vs, results)
	js, err := backtest.JSON(results)
	if err != nil {
		return err
	}
	if *out == "-" {
		fmt.Print(md)
		return nil
	}
	if *out == "" {
		*out = filepath.Join(*dir, "RESULTS.md")
	}
	if err := os.WriteFile(*out, []byte(md), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(filepath.Dir(*out), "results.json"), append(js, '\n'), 0o644)
}
