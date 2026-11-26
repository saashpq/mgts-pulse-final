// calibrate — имитационная калибровка порогов S1/S2: CSV и markdown (docs/calibration.md).
//
//	go run ./cmd/calibrate [-seed 1] [-n 3000] [-out docs]
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ElzaraA/mgts-ai/internal/calib"
)

func main() {
	p := calib.DefaultParams()
	flag.Int64Var(&p.Seed, "seed", p.Seed, "seed генератора")
	flag.IntVar(&p.N, "n", p.N, "рядов на сценарий")
	flag.Float64Var(&p.ItemSD, "noise", p.ItemSD, "шум пункта (SD)")
	flag.Float64Var(&p.Prevalence, "prevalence", p.Prevalence, "доля пар с проблемой для E[C]")
	out := flag.String("out", "docs", "каталог для calibration.md и calibration.csv")
	flag.Parse()
	res := calib.Run(p, calib.Grid())
	if err := write(filepath.Join(*out, "calibration.csv"), res, calib.WriteCSV); err != nil {
		fail(err)
	}
	if err := write(filepath.Join(*out, "calibration.md"), res, calib.WriteMarkdown); err != nil {
		fail(err)
	}
	fmt.Printf("σ̂ = %.3f; записано: %s/calibration.{md,csv}\n", res.Sigma, *out)
}

func write(path string, res calib.Result, fn func(w io.Writer, r calib.Result) error) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := fn(f, res); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "calibrate:", err)
	os.Exit(1)
}
