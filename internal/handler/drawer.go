package handler

import "net/http"

// drawerSavedEvent closes the drawer (components.Drawer) whose form got
// saved, sent as HX-Trigger on the answer.
const drawerSavedEvent = "drawer-saved"

// addHXTrigger adds ev to the answer's HX-Trigger events. Like every header,
// it must be set before writing to the body (e.g. out of band swaps).
func addHXTrigger(w http.ResponseWriter, ev string) {
	if cur := w.Header().Get("HX-Trigger"); cur != "" {
		ev = cur + ", " + ev
	}
	w.Header().Set("HX-Trigger", ev)
}

// drawerSaved closes the drawer of an edit form that got saved, swapping the
// answer into target with swap (e.g. the entry's card, outerHTML) instead of
// the form; an empty target keeps the form's.
func drawerSaved(w http.ResponseWriter, target, swap string) {
	if target != "" {
		w.Header().Set("HX-Retarget", target)
		w.Header().Set("HX-Reswap", swap)
	}
	addHXTrigger(w, drawerSavedEvent)
}

// drawerCreated closes the drawer of a create form (form.SheetForm) that got
// saved, swapping the fresh form the answer is in once it slid out.
func drawerCreated(w http.ResponseWriter) {
	w.Header().Set("HX-Reswap", "outerHTML swap:200ms")
	addHXTrigger(w, drawerSavedEvent)
}
