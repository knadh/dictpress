// Search form.
(function() {
  var form = document.querySelector(".search-form");
  if (!form) {
    return false;
  }


  // Capture the form submit and send it as a canonical URL instead
  // of the ?q query param. 
  var isOn = false;
  function search() {
    // The autocomplete suggestion click doesn't fire a submit, but Enter
    // fires a submit. So to avoid double submits in autcomplete.onSelect(),
    // add a debounce.
    if (isOn) {
      return false;
    }
    isOn = true;
    window.setTimeout(function() {
      isOn = false;
    }, 50);

    var f = form.querySelector("input[name='q']");
    if (!f) {
      return false;
    }
    var q = encodeURIComponent(f.value.replace(/\s+/g, " ").trim()).replace(/%20/g, "+");
    document.location.href = form.getAttribute("action") + "/" + q;
    return false;
  }

  // Bind to form submit.
  form.addEventListener("submit", function(e) {
    e.preventDefault();
    search();
  });
})();


// Submission form.
(() => {
  function filterTypes(e) {
    // Filter the types select field with elements that are supported by the language.
    const types = e.target.closest("fieldset").querySelector("select[name=relation_type]");
    types.querySelectorAll("option").forEach((o) => o.style.display = "none");
    types.querySelectorAll(`option[data-lang=${e.target.value}]`).forEach((o) => o.style.display = "block");
    types.selectedIndex = 1;
  }


  if (document.querySelector(".form-submit")) {
    document.querySelectorAll("select[name=relation_lang]").forEach((e) => {
      e.onchange = filterTypes;
    });

    // +definition button.
    document.querySelector(".btn-add-relation").onclick = (e) => {
      e.preventDefault();

      if (document.querySelectorAll(".add-relations li").length >= 20) {
        return false;
      }

      // Clone and add a relation fieldset.
      const d = document.querySelector(".add-relations li").cloneNode(true);
      d.dataset.added = true
      d.querySelector("select[name=relation_lang]").onchange = filterTypes;
      document.querySelector(".add-relations").appendChild(d);

      // Remove definition link.
      d.querySelector(".btn-remove-relation").onclick = (e) => {
        e.preventDefault();
        d.remove();
      };
    };
  }
})();

// Edit form.
(() => {
  document.querySelectorAll(".edit").forEach((o) => {
    o.onclick = ((e) => {
      e.preventDefault();
      const btn = e.target;

      // Form is already open.
      if (btn.dataset.open) {
        return;
      }
      btn.dataset.open = true;

      const form = document.querySelector(".form-comments").cloneNode(true);
      o.parentNode.appendChild(form);
      form.style.display = "block";

      const txt = form.querySelector("textarea");
      txt.focus();
      txt.onkeydown = (e) => {
        if (e.key === "Escape") {
          close();
        }
      };

      const close = () => {
        btn.dataset.open = "";
        form.remove();
      };

      // Handle form submission.
      form.onsubmit = () => {
        fetch("/api/submissions/comments", {
          method: "POST",
          headers: {
            "Content-Type": "application/json"
          },
          body: JSON.stringify({
            from_guid: btn.dataset.from,
            to_guid: btn.dataset.to,
            comments: txt.value
          })
        }).catch((err) => {
          alert(`Error submitting: ${err}`);
        });

        close();
        alert("Submitted for review");
      };

      form.querySelector("button.close").onclick = close;
    });
  })
})();