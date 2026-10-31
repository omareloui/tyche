// moneyInputs backs a price form (see views.purchaseInputs): the amounts'
// mask follows the picked currency's symbol, and paying in installments takes
// their months or amount per month, working out the other from the price
// like the server does.
document.addEventListener("alpine:init", () => {
  Alpine.data("moneyInputs", (syms = {}, sym = "", mode = "cash") => ({
    syms,
    sym,
    mode,
    price: 0,
    months: 0,
    per: 0,

    init() {
      this.read();
    },

    num(v) {
      return parseFloat(String(v ?? "").replace(/[^0-9.]/g, "")) || 0;
    },

    money(n) {
      const s = n.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 });
      return this.sym ? `${this.sym} ${s}` : s;
    },

    field(name) {
      return this.$root.querySelector(`[name="${name}"]`);
    },

    read() {
      this.price = this.num(this.field("amount")?.value);
      this.months = Math.floor(this.num(this.field("installments")?.value));
      this.per = this.num(this.field("installment_amount")?.value);
    },

    onChange(e) {
      if (e.target.name === "mode") this.mode = e.target.value;
      if (e.target.name !== "currency_id") return;
      this.sym = this.syms[e.target.value] ?? this.sym;
      // Re-mask the amounts with the new symbol.
      this.$nextTick(() => {
        this.$root.querySelectorAll("[data-money]").forEach((i) => {
          if (i.value) i.dispatchEvent(new Event("input", { bubbles: true }));
        });
      });
    },

    // plan is the installments as they'll be counted: [months, per month].
    get plan() {
      const { price, months, per } = this;
      if (months > 0 && per > 0) return [months, per];
      if (months > 0 && price > 0) return [months, price / months];
      if (per > 0 && price > 0) {
        const n = Math.ceil(Math.floor((price / per) * 100) / 100);
        return [n, price / n];
      }
      return null;
    },

    get planText() {
      const plan = this.plan;
      if (!plan) return "Fill in the months or the amount per month, the other is worked out from the price.";
      const [n, per] = plan;
      let text = `${n} ${n === 1 ? "month" : "months"} × ${this.money(per)} = ${this.money(n * per)}`;
      const extra = n * per - this.price;
      if (this.price > 0 && extra > 0.005) text += `, ${this.money(extra)} over the price`;
      return text;
    },
  }));
});
