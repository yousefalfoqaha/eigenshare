function formatBytes(bytes) {
  const unit = 1024;
  if (bytes < unit) {
    return `${bytes} B`;
  }
  let div = unit;
  let exp = 0;
  for (let n = bytes / unit; n >= unit; n /= unit) {
    div *= unit;
    exp++;
  }
  return `${(bytes / div).toFixed(1)} ${"KMGTPE"[exp]}B`;
}

async function registerFiles(shareID, files) {
  const response = await fetch(`/api/shares/${shareID}/files`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      files: files.map((f) => ({
        name: f.name,
        expectedSize: f.size,
      })),
    }),
  });

  if (!response.ok) {
    const error = await response.json().catch(() => ({}));
    throw new Error(
      error.message || `failed to register files: ${response.status}`,
    );
  }

  return (await response.json()) || [];
}

async function streamFile(shareID, fileID, file, onProgress) {
  const chunkSize = 1024 * 1024;
  let offset = 0;

  while (offset < file.size) {
    const end = Math.min(offset + chunkSize, file.size);
    const response = await fetch(
      `/api/shares/${shareID}/files/${fileID}/stream`,
      {
        method: "PUT",
        headers: {
          "Content-Type": "application/octet-stream",
          "Upload-Offset": String(offset),
        },
        body: await file.slice(offset, end).arrayBuffer(),
      },
    );

    if (!response.ok) {
      const uploadOffset = Number(response.headers.get("Upload-Offset"));
      if (
        response.status === 409 &&
        Number.isSafeInteger(uploadOffset) &&
        uploadOffset > offset &&
        uploadOffset <= file.size
      ) {
        offset = uploadOffset;
        if (onProgress) onProgress(offset);
        continue;
      }
      const error = await response.json().catch(() => ({}));
      throw new Error(error.message || `upload failed: ${response.status}`);
    }

    const nextOffset = Number(response.headers.get("Upload-Offset"));
    if (
      !Number.isSafeInteger(nextOffset) ||
      nextOffset <= offset ||
      nextOffset > file.size
    ) {
      throw new Error("invalid Upload-Offset response header");
    }
    offset = nextOffset;
    if (onProgress) onProgress(offset);
  }
}

function wirePicker(root) {
  const input = root.querySelector("[data-file-input]");
  const selectButton = root.querySelector("[data-file-select]");
  const list = root.querySelector("[data-file-list]");
  const files = [];

  function renderFiles() {
    list.replaceChildren();

    files.forEach((file, index) => {
      const row = document.createElement("li");
      row.className = "file-picker-row";

      const name = document.createElement("span");
      name.className = "truncate";
      name.title = file.name;
      name.textContent = file.name;

      const removeButton = document.createElement("button");
      removeButton.type = "button";
      removeButton.textContent = "remove";
      removeButton.addEventListener("click", () => {
        files.splice(index, 1);
        renderFiles();
      });

      row.append(name, removeButton);
      list.appendChild(row);
    });
  }

  selectButton.addEventListener("click", () => input.click());

  input.addEventListener("change", () => {
    files.push(...input.files);
    input.value = "";
    renderFiles();
  });

  return { files: () => [...files] };
}

function createQueue(root) {
  const statusEl = root.querySelector("[data-status]");
  const linesEl = root.querySelector("[data-lines]");

  const entries = [];
  const progress = {};
  const listed = new Set();

  function written(entry) {
    return progress[entry.id] || 0;
  }

  function isDone(entry) {
    return written(entry) >= entry.expectedSize;
  }

  function incomplete() {
    return entries.filter((entry) => !isDone(entry));
  }

  function stateOf(entry, activeID) {
    if (entry.id === activeID) return "uploading";
    if (isDone(entry)) return "done";
    if (written(entry) > 0) return "paused";
    return "pending";
  }

  function setStatus(text) {
    statusEl.textContent = text;
  }

  function lineFor(entry, activeID) {
    const state = stateOf(entry, activeID);

    const li = document.createElement("li");
    li.dataset.state = state;

    const head = document.createElement("div");
    head.className = "line-head";

    const name = document.createElement("span");
    name.className = "truncate";
    name.title = entry.name;
    name.textContent = entry.name;

    const stateEl = document.createElement("span");
    stateEl.className = "state";
    stateEl.textContent = state;

    head.append(name, stateEl);

    const meter = document.createElement("div");
    meter.className = "line-meter";

    const bar = document.createElement("progress");
    bar.max = entry.expectedSize || 1;
    bar.value = isDone(entry) ? bar.max : written(entry);

    const bytes = document.createElement("span");
    bytes.className = "bytes";
    bytes.textContent = `${formatBytes(written(entry))} / ${formatBytes(entry.expectedSize)}`;

    meter.append(bar, bytes);

    li.append(head, meter);
    return li;
  }

  function renderLines(activeID = null) {
    linesEl.replaceChildren(
      ...entries
        .filter((entry) => listed.has(entry.id))
        .map((entry) => lineFor(entry, activeID)),
    );
  }

  function seed(newEntries, progressMap) {
    entries.push(...(newEntries || []));
    Object.assign(progress, progressMap || {});
    for (const entry of incomplete()) listed.add(entry.id);
    renderLines();
  }

  async function registerPairs(shareID, files) {
    const newEntries = await registerFiles(shareID, files);
    entries.push(...newEntries);
    for (const entry of newEntries) listed.add(entry.id);
    renderLines();
    return newEntries.map((entry, i) => ({ entry, file: files[i] }));
  }

  async function uploadFiles(shareID, pairs) {
    setStatus("");
    for (const { entry, file } of pairs) {
      renderLines(entry.id);
      await streamFile(shareID, entry.id, file, (bytesWritten) => {
        progress[entry.id] = bytesWritten;
        renderLines(entry.id);
      });
    }
  }

  function fail(message) {
    setStatus(message);
    renderLines();
  }

  return { seed, registerPairs, incomplete, uploadFiles, fail };
}

function createUploader(root, opts = {}) {
  const form = root.closest("form");
  const button = root.querySelector("[data-upload-btn]");
  const picker = wirePicker(root);
  const queue = createQueue(root);

  function setBusy(busy) {
    button.disabled = busy;
  }

  async function submitAll(shareID) {
    const selected = picker.files();
    if (selected.length === 0) return;

    const pairs = [];
    const fresh = [];
    const claimed = new Set();
    const open = queue.incomplete();

    for (const file of selected) {
      const entry = open.find(
        (e) =>
          !claimed.has(e.id) &&
          e.name === file.name &&
          e.expectedSize === file.size,
      );
      if (entry) {
        claimed.add(entry.id);
        pairs.push({ entry, file });
      } else {
        fresh.push(file);
      }
    }

    if (fresh.length > 0) {
      pairs.push(...(await queue.registerPairs(shareID, fresh)));
    }

    await queue.uploadFiles(shareID, pairs);
  }

  function autoSubmit(shareID) {
    form.addEventListener("submit", async (e) => {
      e.preventDefault();
      if (picker.files().length === 0) return;

      setBusy(true);
      try {
        await submitAll(shareID);
        window.location.reload();
      } catch (err) {
        queue.fail(err.message);
        setBusy(false);
      }
    });
  }

  if (opts.shareID) {
    queue.seed(opts.files, opts.progress);
    autoSubmit(opts.shareID);
  }

  return {
    files: picker.files,
    submitAll,
    setBusy,
    fail: queue.fail,
  };
}
