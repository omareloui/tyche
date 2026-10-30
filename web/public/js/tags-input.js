// tagsInput edits a comma separated list as chips (see form.TagsInput). The
// hidden input keeps the list joined by ", " for the form to submit; Enter,
// comma or leaving the field adds the draft, Backspace on an empty draft
// removes the last chip.
document.addEventListener("alpine:init", () => {
  Alpine.data("tagsInput", () => ({
    tags: [],
    draft: "",

    get value() {
      return this.tags.join(", ");
    },

    init() {
      this.tags = this.$refs.input.value
        .split(",")
        .map((t) => t.trim())
        .filter(Boolean);
    },

    add() {
      for (const part of this.draft.split(",")) {
        const tag = part.trim();
        const exists = this.tags.some(
          (t) => t.toLowerCase() === tag.toLowerCase(),
        );
        if (tag && !exists) this.tags.push(tag);
      }
      this.draft = "";
    },

    remove(i) {
      this.tags.splice(i, 1);
      this.$refs.draft.focus();
    },

    backspace() {
      if (this.draft === "") this.tags.pop();
    },
  }));
});
