package tkb

import (
	"fmt"
	"math/big"
)

type Percent int64

const hundredPercent Percent = 10000

func Ratio(part, total int64) Percent {
	if total <= 0 {
		return 0
	}
	num := new(big.Int).Mul(big.NewInt(part), big.NewInt(int64(hundredPercent)*2))
	num.Add(num, big.NewInt(total))
	den := new(big.Int).Mul(big.NewInt(total), big.NewInt(2))
	return Percent(new(big.Int).Quo(num, den).Int64())
}

func (p Percent) String() string {
	sign := ""
	v := int64(p)
	if v < 0 {
		sign = "-"
		v = -v
	}
	return fmt.Sprintf("%s%d.%02d", sign, v/100, v%100)
}

func (p Percent) MarshalJSON() ([]byte, error) {
	return []byte(p.String()), nil
}

func (p *Percent) UnmarshalJSON(b []byte) error {
	r, ok := new(big.Rat).SetString(string(b))
	if !ok {
		return fmt.Errorf("invalid percent %s", b)
	}
	r.Mul(r, big.NewRat(100, 1))
	if !r.IsInt() {
		return fmt.Errorf("percent %s has more than two decimals", b)
	}
	*p = Percent(r.Num().Int64())
	return nil
}
