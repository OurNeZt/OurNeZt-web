(() => {
  // Match Unicode White_Space, including nonbreaking spaces and line separators.
  const whitespace = /[\t-\r \u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]/u;
  const invalidMessage = "Remove spaces or other whitespace from this password.";
  const blockedMessage = "Spaces and other whitespace aren't allowed. Nothing was added.";
  const forms = new Map();

  document.querySelectorAll("input[data-password-no-whitespace]").forEach((input, index) => {
    if (input.dataset.passwordWhitespaceReady) return;
    input.dataset.passwordWhitespaceReady = "true";

    const message = document.createElement("span");
    message.id = `password-whitespace-${index + 1}`;
    message.className = "text-sm text-error";
    message.hidden = true;
    message.setAttribute("role", "status");
    message.setAttribute("aria-live", "polite");
    input.insertAdjacentElement("afterend", message);
    input.setAttribute("aria-describedby", [input.getAttribute("aria-describedby"), message.id].filter(Boolean).join(" "));

    const show = (text) => {
      message.textContent = text;
      message.hidden = text === "";
    };
    const validate = () => {
      const invalid = whitespace.test(input.value);
      input.setCustomValidity(invalid ? invalidMessage : "");
      input.setAttribute("aria-invalid", String(invalid));
      show(invalid ? invalidMessage : "");
      return !invalid;
    };
    const block = (event) => {
      if (!event.cancelable) return;
      event.preventDefault();
      // Keep the existing password untouched; never remove characters silently.
      const valid = validate();
      if (valid) show(blockedMessage);
    };

    input.addEventListener("keydown", (event) => {
      // Space can be part of IME composition; validate its final text instead.
      if (event.key === " " && !event.isComposing) block(event);
    });
    input.addEventListener("beforeinput", (event) => {
      if (!event.isComposing && whitespace.test(event.data || "")) block(event);
    });
    input.addEventListener("paste", (event) => {
      if (whitespace.test(event.clipboardData?.getData("text") || "")) block(event);
    });
    input.addEventListener("drop", (event) => {
      if (whitespace.test(event.dataTransfer?.getData("text") || "")) block(event);
    });
    input.addEventListener("input", validate);
    input.addEventListener("change", validate);
    input.addEventListener("compositionend", validate);
    validate();

    if (input.form) {
      if (!forms.has(input.form)) forms.set(input.form, []);
      forms.get(input.form).push({ input, validate });
    }
  });

  forms.forEach((fields, form) => {
    form.addEventListener("submit", (event) => {
      const invalid = fields.filter((field) => !field.validate());
      if (invalid.length === 0) return;
      event.preventDefault();
      invalid[0].input.focus();
      invalid[0].input.reportValidity();
    });
  });
})();
