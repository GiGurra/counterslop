package game

import (
	"math"
	"testing"
)

// The garage's numbers (car_shared.go Fit*, shown by web/game/garage.js and
// fitted by fitCar / refillCharges / carTank) are the old rounding of the
// server's fit for every type, item and tier.
func TestFitNumbers(t *testing.T) {
	if len(CarTypes) == 0 || len(CarItems) == 0 {
		t.Fatal("cars.json not loaded (TestMain)")
	}
	for tier := 0; tier <= 3; tier++ {
		for _, sp := range CarTypes {
			if got, want := FitMaxHP(sp.HP, tier, CarShop.Plating), int(math.Round(float64(sp.HP)*(1+CarShop.Plating*float64(tier)))); got != want {
				t.Errorf("%s plating %d: HP %d, want %d", sp.ID, tier, got, want)
			}
			if got, want := FitTopSpeed(sp.TopSpeed, tier), sp.TopSpeed*(1+0.02*float64(tier)); math.Abs(got-want) > 1e-9 {
				t.Errorf("%s engine %d: top %v, want %v", sp.ID, tier, got, want)
			}
		}
		for _, it := range CarItems {
			if got, want := FitCharges(it.Charges, tier, CarShop.ChargePack), int(math.Round(float64(it.Charges)*(1+CarShop.ChargePack*float64(tier)))); got != want {
				t.Errorf("%s packs %d: %d charges, want %d", it.ID, tier, got, want)
			}
		}
		if got, want := carTank(garage{Tank: tier}), CarShop.Boost+CarShop.Tank*float64(tier); got != want {
			t.Errorf("tank %d: %v s, want %v", tier, got, want)
		}
	}
	if FitMaxHP(250, 2, 0.25) != 375 || FitCharges(3, 1, 0.5) != 5 || FitTank(2.5, 1.5, 2) != 5.5 {
		t.Error("spot values (web/game/garage.test.mjs pins the same)")
	}
}
