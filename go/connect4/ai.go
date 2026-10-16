package connect4

import "math/rand/v2"

// Search order: centre columns first, which both plays better and makes
// alpha-beta pruning far more effective.
var columnOrder = [Cols]int{3, 2, 4, 1, 5, 0, 6}

const winScore = 1_000_000

// bestMove picks a column for me, looking depth moves ahead.
func bestMove(b Board, me Disc, depth int) int {
	best, bestScore := -1, -winScore*2
	var ties []int
	for _, col := range columnOrder {
		row := b.dropRow(col)
		if row < 0 {
			continue
		}
		b[row][col] = me
		var score int
		if _, won := b.winAt(row, col); won {
			score = winScore + depth // win sooner rather than later
		} else {
			score = -negamax(&b, me.other(), depth-1, -winScore*2, winScore*2)
		}
		b[row][col] = Empty
		switch {
		case score > bestScore:
			best, bestScore, ties = col, score, []int{col}
		case score == bestScore:
			ties = append(ties, col)
		}
	}
	// Vary play between equally good moves so games aren't identical.
	if len(ties) > 1 {
		return ties[rand.IntN(len(ties))]
	}
	return best
}

// negamax returns the score of the position for the player to move.
func negamax(b *Board, me Disc, depth, alpha, beta int) int {
	if b.full() {
		return 0
	}
	if depth == 0 {
		return evaluate(b, me)
	}
	best := -winScore * 2
	for _, col := range columnOrder {
		row := b.dropRow(col)
		if row < 0 {
			continue
		}
		b[row][col] = me
		var score int
		if _, won := b.winAt(row, col); won {
			score = winScore + depth
		} else {
			score = -negamax(b, me.other(), depth-1, -beta, -alpha)
		}
		b[row][col] = Empty
		best = max(best, score)
		alpha = max(alpha, score)
		if alpha >= beta {
			break
		}
	}
	return best
}

// evaluate scores a position for me by looking at every line of four cells:
// lines only one player can still complete are worth more the fuller they are.
func evaluate(b *Board, me Disc) int {
	score := 0
	for r := 0; r < Rows; r++ {
		if b[r][Cols/2] == me {
			score += 3 // the centre column takes part in the most lines
		} else if b[r][Cols/2] == me.other() {
			score -= 3
		}
	}
	for r := 0; r < Rows; r++ {
		for c := 0; c < Cols; c++ {
			for _, d := range directions {
				if !in(r+3*d[0], c+3*d[1]) {
					continue
				}
				mine, theirs := 0, 0
				for i := 0; i < 4; i++ {
					switch b[r+i*d[0]][c+i*d[1]] {
					case me:
						mine++
					case me.other():
						theirs++
					}
				}
				score += lineValue(mine, theirs) - lineValue(theirs, mine)
			}
		}
	}
	return score
}

func lineValue(mine, theirs int) int {
	if theirs > 0 {
		return 0
	}
	switch mine {
	case 3:
		return 50
	case 2:
		return 5
	}
	return 0
}
