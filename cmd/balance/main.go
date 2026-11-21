// Command balance prints the time-to-kill tables computed from the weapon
// data (internal/game/balance.go) as markdown, for the CI job summary:
//
//	go run ./cmd/balance               print
//	go run ./cmd/balance -md out.md    also append it to out.md
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/gigurra/counterslop/internal/game"
)

func main() {
	md := flag.String("md", "", "also append the tables to this file")
	flag.Parse()
	wb, err := os.ReadFile("web/assets/weapons/weapons.json")
	if err != nil {
		log.Fatal(err)
	}
	reg, err := game.LoadWeapons(wb)
	if err != nil {
		log.Fatal(err)
	}
	mb, err := os.ReadFile("web/assets/models/player.json")
	if err != nil {
		log.Fatal(err)
	}
	if err := game.LoadModel(mb); err != nil {
		log.Fatal(err)
	}
	var econ *game.Economy
	if eb, err := os.ReadFile("web/assets/economy.json"); err == nil {
		econ, _ = game.LoadEconomy(eb)
	}
	out := game.BalanceMarkdown(reg, econ, game.BalanceTable(reg, game.PlayerModel.Mult))
	fmt.Print(out)
	if *md != "" {
		f, err := os.OpenFile(*md, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		f.WriteString(out)
	}
}
