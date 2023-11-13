(() => {
  const elTabs = document.querySelectorAll(`.tabs input`);
  const elForm = document.querySelector("form.search-form");
  const elQ = document.querySelector("#q");
  const defaultLang = document.querySelector(`.tabs input:first-child`).value;

  function selectDict(dict) {
    // dict is in the format '$fromLang/$toLang'.
    const langs = dict.split("/");
    const t = document.querySelector(`.tabs #tab-${langs[0]}-${langs[1]}`);
    if (!t) {
      return;
    }

    Object.assign(localStorage, { dict, from_lang: langs[0], to_lang: langs[1] });

    t.checked = true;
    elForm.setAttribute("action", `${_ROOT_URL}/dictionary/${dict}`);

    elQ.focus();
    elQ.select();
  }

  // Capture the form submit and send it as a canonical URL instead
  // of the ?q query param. 
  function search(q) {
    let val = q.trim();

    const uri = elForm.getAttribute("action");
    document.location.href = `${uri}/${encodeURIComponent(val).replace(/%20/g, "+")}`;
  }

  // On ~ press, focus search input.
  document.onkeydown = (function (e) {
    if (e.keyCode != 192) {
      return;
    }

    e.preventDefault();
    q.focus();
    q.select();
  });

  // On language tab selector click.
  elTabs.forEach((el) => {
    el.onchange = (e) => {
      e.preventDefault();
      selectDict(e.target.value);
    }
  });

  // Bind to form submit.
  elForm.addEventListener("submit", function (e) {
    e.preventDefault();
    search(elQ.value);
  });


  // Select a language based on the page URL.
  let dict = localStorage.dict || defaultLang;
  const uri = /(dictionary)\/((.+?)\/(.+?))\//i.exec(document.location.href);
  if (uri && uri.length == 5) {
    dict = uri[2];
  }

  selectDict(dict);
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
      if (btn.close) {
        btn.close();
        return;
      }

      const form = document.querySelector(".form-comments").cloneNode(true);
      o.parentNode.appendChild(form);
      form.style.display = "block";

      const txt = form.querySelector("textarea");
      txt.focus();
      txt.onkeydown = (e) => {
        if (e.key === "Escape") {
          btn.close();
        }
      };

      btn.close = () => {
        btn.close = null;
        form.remove();
      };

      // Handle form submission.
      form.onsubmit = () => {
        fetch(`${window._ROOT_URL}/api/submissions/comments`, {
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

        alert(form.dataset.success);
        btn.close();
      };

      form.querySelector("button.close").onclick = btn.close;
    });
  })
})();
