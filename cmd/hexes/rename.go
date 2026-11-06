package main

// Renaming a hex: tap the name on its card. On the web an input box
// comes up over the game so the phone's keyboard shows; on a desktop
// build the name is typed straight into the game.

type renameResult struct {
	i    int
	name string
	ok   bool // false when the rename was cancelled
}

// applyRename puts a typed name on hex i. A blank one keeps the old name.
func (g *Game) applyRename(i int, name string) {
	got, err := g.sim.RenameHex(i, name)
	if err != nil {
		g.ed.msg, g.ed.bad = err.Error(), true
		g.click(false)
		return
	}
	g.ed.msg, g.ed.bad = "Named it "+got+".", false
	if g.sheetHex == i {
		if s, err := g.sim.HexCard(i); err == nil {
			g.sheet = s
		}
	}
	g.click(true)
}

// pollRename applies a rename that came back from the input box.
func (g *Game) pollRename() {
	select {
	case r := <-g.renameCh:
		g.renaming = -1
		if r.ok {
			g.applyRename(r.i, r.name)
		}
	default:
	}
}
