// moneyInputs backs a price form (see views.purchaseInputs): the amounts'
// mask follows the picked currency's symbol, or a percentage for fields with
// a unit toggle set to %, and it tracks the payment mode, the installment
// strategy and what a known amount is given by (months or per month) to show
// their fields.
document.addEventListener("alpine:init", () => {
  Alpine.data("moneyInputs", (syms = {}, sym = "", mode = "cash", strategy = "known") => ({
    syms,
    sym,
    mode,
    strategy,
    termBy: "months",

    // tick changes with every edit, for the bindings reading the inputs.
    tick: 0,

    init() {
      this.termBy = this.$root.querySelector('[name="term_by"]:checked')?.value ?? "months";
      const bump = () => this.tick++;
      this.$root.addEventListener("input", bump);
      this.$root.addEventListener("change", bump);
    },

    // num reads a masked amount or percentage.
    num(v) {
      return parseFloat(String(v ?? "").replace(/[^0-9.]/g, "")) || 0;
    },

    // pctOfPrice is a field's percentage of the price as an amount, "" when
    // it's not a percentage. The price is the form's, or the scope's
    // data-price (a goal's).
    pctOfPrice(name) {
      this.tick;
      if (this.unitOf(`${name}_unit`) !== "pct") return "";
      const pct = this.num(this.$root.querySelector(`[name="${name}"]`)?.value);
      const price = this.$root.dataset.price
        ? this.num(this.$root.dataset.price)
        : this.num(this.$root.querySelector('[name="amount"]')?.value);
      return pct && price ? this.money((price * pct) / 100) : "";
    },

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
      if (name === "term_by") this.termBy = e.target.value;
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
