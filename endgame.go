package main

func getDrawishness(p *Pos, strong, weak int) int {

	if p.count[strong][P] == 0 {

		// K vs K, KB vs K, KN v K, KB vs KP(P), KN vs KP(P)
		if p.count[strong][Q] + p.count[strong][R] == 0 &&
		   p.count[strong][B] + p.count[strong][N] < 2 {
			return 0
		   }

		// KR vs KB(P), KR vs KN(P)
		if p.count[strong][Q] == 0 && p.count[strong][R] == 1 &&
		   p.count[strong][B] + p.count[strong][N] == 0 &&
		   p.count[weak][Q] + p.count[weak][R] == 0 && 
		    p.count[weak][B] + p.count[weak][N] == 1 {
			return 25
		   }

		// KRB vs KR(P), KRN vs KR(P)
		if p.count[strong][Q] == 0 && p.count[strong][R] == 1 &&
		   p.count[strong][B] + p.count[strong][N] == 1 &&
		   p.count[weak][Q] == 0 && p.count[weak][R] == 1 && 
		    p.count[weak][B] + p.count[weak][N] == 0 {
			return 25
		   }


	}

	return 100
}

