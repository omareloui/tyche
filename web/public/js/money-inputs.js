// moneyInputs backs a price form (see views.purchaseInputs): the amounts'
// mask follows the picked currency's symbol, or a percentage for fields with
// a unit toggle set to %, and it tracks the payment mode and the installment
// strategy to show their fields.
document.addEventListener("alpine:init", () => {
  Alpine.data("moneyInputs", (syms = {}, sym = "", mode = "cash", strategy = "known") => ({
    syms,
    sym,
    mode,
    strategy,

    money(n) {
      const s = n.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 });
      return this.sym ? `${this.sym} ${s}` : s;
    },

    // unitOf is the picked unit of a unit toggle: "amount" or "pct".
    unitOf(name) {
      return this.$root.querySelector(`[name="${name}"]:checked`)?.value ?? "amount";
    },

    remask(inputs) {
      this.$nextTick(() => {
        inputs.forEach((i) => {
          if (i.value) i.dispatchEvent(new Event("input", { bubbles: true }));
        });
      });
    },

    onChange(e) {
      const name = e.target.name ?? "";
      if (name === "mode") this.mode = e.target.value;
      if (name === "strategy") this.strategy = e.target.value;
      if (name.endsWith("_unit")) {
        this.remask(this.$root.querySelectorAll(`[name="${name.slice(0, -"_unit".length)}"]`));
      }
      if (name === "currency_id") {
        this.sym = this.syms[e.target.value] ?? this.sym;
        // Re-mask the amounts with the new symbol.
        this.remask(this.$root.querySelectorAll("[data-money]"));
      }
    },
  }));
});
