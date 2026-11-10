// Command gencommon builds data/common.txt, the easy bot's word list, from SCOWL
// size lists and the ENABLE list.
//
//	go run ./cmd/gencommon -enable data/enable1.txt -out data/common.txt \
//	    final/english-words.10 final/english-words.20 final/english-words.35 \
//	    final/american-words.10 final/american-words.20 final/american-words.35
//
// Only words in ENABLE are kept, because the bot can't play anything else.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
)

func main() {
	enablePath := flag.String("enable", "data/enable1.txt", "ENABLE word list")
	outPath := flag.String("out", "data/common.txt", "output file")
	flag.Parse()

	enable, err := readWords(*enablePath)
	if err != nil {
		log.Fatal(err)
	}
	allowed := make(map[string]bool, len(enable))
	for _, word := range enable {
		allowed[word] = true
	}

	var common []string
	for _, path := range flag.Args() {
		words, err := readWords(path)
		if err != nil {
			log.Fatal(err)
		}
		for _, word := range words {
			if allowed[word] {
				common = append(common, word)
			}
		}
	}
	slices.Sort(common)
	common = slices.Compact(common)

	if err := os.WriteFile(*outPath, []byte(strings.Join(common, "\n")+"\n"), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%d common words written to %s\n", len(common), *outPath)
}

func readWords(path string) ([]string, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(text)), nil
}
