function selectDict(dict) {
  const langs = dict.split("/");
  localStorage.dict = dict;
  localStorage.from_lang = langs[0];
  localStorage.to_lang = langs[1];

  document.querySelectorAll(`.tabs .tab`).forEach((el) => el.classList.remove("sel"));
  document.querySelector(`.tabs .tab[data-dict='${dict}']`).classList.add("sel");
  document.querySelector("form.search-form").setAttribute("action", `/dictionary/${dict}`);
  const q = document.querySelector("#q");
  q.focus();
  q.select();
}

// Capture the form submit and send it as a canonical URL instead
// of the ?q query param. 
function search(q) {
  let val = q.trim().toLowerCase().replace(/\s+/g, ' ');

  const uri = document.querySelector(".search-form").getAttribute("action");
  document.location.href = `${uri}/${encodeURIComponent(val).replace(/%20/g, "+")}`;
}


(() => {
  // Search input.
  const q = document.querySelector("#q");

  // On ~ press, focus search input.
  document.onkeydown = (function (e) {
    if (e.keyCode != 192) {
      return;
    }

    e.preventDefault();
    q.focus();
    q.select();
  });

  // Select a language tab on page load.
  let dict = document.querySelector(`.tabs .tab:first-child`).dataset.dict;
  if (localStorage.dict && document.querySelector(`.tabs .tab[data-dict='${localStorage.dict}']`)) {
    dict = localStorage.dict;
  }
  selectDict(dict);

  // On language tab selector click.
  document.querySelectorAll(`.tabs .tab`).forEach((el) => {
    el.onclick = (e) => {
      e.preventDefault();
      selectDict(e.target.dataset.dict);
    }
  });

  // Bind to form submit.
  document.querySelector(".search-form").addEventListener("submit", function (e) {
    e.preventDefault();
    search(document.querySelector("#q").value);
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
        close();
      };

      form.querySelector("button.close").onclick = close;
    });
  })
})();