export function autocomp(el, options = {}) {
	const opt = {
		onQuery: null, onNavigate: null, onSelect: null, onRender: null, debounce: 100, autoSelect: true,...options
	};

	let box, cur = opt.autoSelect ? 0 : -1, items = [], val, req;

	// Disable browser's default autocomplete behaviour on the input.
	el.autocomplete = "off";

	// Attach all the events required for the interactions in one go.
	["input", "keydown", "blur"].forEach(k => el.addEventListener(k, handleEvent));

	function handleEvent(e) {
		if (e.type === "keydown" && !handleKeydown(e)) {
			return;
		}

		if (e.type === "blur") {
			return destroy();
		}

		const newVal = e.target.value;
		if (!newVal) {
			destroy();
			val = null;
			return;
		}

		if (newVal === val && box) {
			return;
		}
		val = newVal;

		// Clear (debounce) any existing pending requests and queue
		// the next search request.
		clearTimeout(req);
		req = setTimeout(query, opt.debounce);
	}

	function handleKeydown(e) {
		if (!box) {
			return e.keyCode === 38 || e.keyCode === 40
		}

		switch (e.keyCode) {
			case 38: return navigate(-1, e); // Up arrow.
			case 40: return navigate(1, e); // Down arrow
			case 9: // Tab
			case 13: // Enter
				e.preventDefault();
				select(cur);
				destroy();
				return;
			case 27: // Escape.
				destroy();
				return;
		}
	}

	async function query() {
		if (!val) {
			return;
		}

		items = await opt.onQuery(val);
		if (!items.length) {
			return destroy();
		}

		if (!box) {
			createBox();
		}

		renderResults();
	}

	function createBox() {
		box = document.createElement("div");
		Object.assign(box.style, {
			width: window.getComputedStyle(el).width,
			position: "absolute",
			left: `${el.offsetLeft}px`,
			top: `${el.offsetTop + el.offsetHeight}px`
		});

		box.classList.add("autocomp");
		el.parentNode.insertBefore(box, el.nextSibling);
	}

	function renderResults() {
		box.innerHTML = "";
		items.forEach((item, idx) => {
			const div = document.createElement("div");
			div.classList.add("autocomp-item");

			// If there's a custom renderer callback, use it, else, simply insert the value/text as-is.
			opt.onRender ? div.appendChild(opt.onRender(item)) : div.innerText = item;
			if (idx === cur) {
				div.classList.add("autocomp-sel");
			}

			div.addEventListener("mousedown", () => select(idx));
			box.appendChild(div);
		});
	}

	function navigate(direction, e) {
		e.preventDefault();

		// Remove the previous item's highlight;
		const prev = box.querySelector(`:nth-child(${cur + 1})`);
		prev?.classList.remove("autocomp-sel");

		// Increment the cursor and highlight the next item, cycled between [0, n].
		cur = (cur + direction + items.length) % items.length;
		box.querySelector(`:nth-child(${cur + 1})`).classList.add("autocomp-sel");
	}

	function select(idx) {
		if (!opt.onSelect) {
			return;
		}

		val = opt.onSelect(items[idx], items);
		el.value = val || items[idx];
	}

	function destroy() {
		items = [];
		cur = opt.autoSelect ? 0 : -1;
		if (box) {
			box.remove();
			box = null;
		}
	}
}

export default autocomp;
