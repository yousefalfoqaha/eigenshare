function formatBytes(bytes) {
  const unit = 1024;
  if (bytes < unit) return `${bytes} B`;
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
      files: files.map((f) => ({ name: f.name, expectedSize: f.size })),
    }),
  });
  if (!response.ok) throw new Error(`failed to register: ${response.status}`);
  return (await response.json()) || [];
}

async function fileState(shareID, fileID, signal) {
  try {
    const res = await fetch(`/api/shares/${shareID}/files/${fileID}/stream`, {
      method: "HEAD",
      signal,
    });
    if (!res.ok) return null;
    const header = res.headers.get("Upload-Offset");
    if (header === null) return null;
    return {
      offset: Number(header),
      locked: res.headers.get("Upload-Locked") !== null,
    };
  } catch (e) {
    return null;
  }
}

function putChunk(shareID, fileID, offset, data, onProgress, signal) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("PUT", `/api/shares/${shareID}/files/${fileID}/stream`);
    xhr.setRequestHeader("Content-Type", "application/octet-stream");
    xhr.setRequestHeader("Upload-Offset", String(offset));

    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable && onProgress) onProgress(offset + e.loaded);
    };

    const fail = (status, message) => {
      const err = new Error(message);
      err.status = status;
      reject(err);
    };

    xhr.onload = () => {
      if (xhr.status === 0) return fail(0, "network error");

      if (xhr.status === 204 || xhr.status === 409) {
        const header = xhr.getResponseHeader("Upload-Offset");
        if (header !== null) return resolve(Number(header));
        return fail(
          xhr.status,
          `Server returned ${xhr.status} but missing Upload-Offset header`,
        );
      }

      let errorMsg = "";
      try {
        const payload = JSON.parse(xhr.responseText);
        if (payload.message) errorMsg = payload.message;
      } catch (e) {}

      fail(
        xhr.status,
        errorMsg
          ? `upload rejected: ${errorMsg}`
          : `upload rejected (HTTP ${xhr.status})`,
      );
    };

    xhr.onerror = () => fail(0, "network error");
    xhr.onabort = () => fail(0, "upload aborted");

    if (signal) {
      signal.addEventListener("abort", () => xhr.abort(), { once: true });
    }
    xhr.send(data);
  });
}

async function streamFile(shareID, fileID, file, onProgress, signal) {
  const chunkSize = 25 * 1024 * 1024;
  let offset = 0;
  let attempts = 0;

  while (offset < file.size) {
    if (signal?.aborted) throw new Error("upload aborted");

    const state = await fileState(shareID, fileID, signal);
    if (state === null) {
      if (++attempts > 3) throw new Error("connection lost. please try again.");
      await new Promise((r) => setTimeout(r, 1000));
      continue;
    }

    offset = state.offset;
    if (onProgress) onProgress(offset);
    if (offset >= file.size) continue;

    if (state.locked) {
      if (++attempts > 3) return "locked";
      await new Promise((r) => setTimeout(r, 1000));
      continue;
    }

    const chunk = file.slice(offset, offset + chunkSize);
    try {
      offset = await putChunk(
        shareID,
        fileID,
        offset,
        chunk,
        onProgress,
        signal,
      );
      if (onProgress) onProgress(offset);
      attempts = 0;
      continue;
    } catch (err) {
      if (signal?.aborted) throw new Error("upload aborted");
      if (
        err.status >= 400 &&
        err.status < 500 &&
        err.status !== 423 &&
        err.status !== 409
      ) {
        throw err;
      }
    }

    if (++attempts > 3) throw new Error("connection lost. please try again.");
    await new Promise((r) => setTimeout(r, 1000));
  }

  return "complete";
}

class UploadClient {
  constructor(shareID, handlers) {
    this.shareID = shareID;
    this.on = handlers;
    this.tasks = new Map();
  }

  #base(id, name, expectedSize, loaded) {
    return {
      id,
      name,
      expectedSize,
      loaded,
      remove: () => this.#deleteFromServer(id),
    };
  }

  INCOMPLETE(id, name, expectedSize, loaded, locked = false) {
    return {
      ...this.#base(id, name, expectedSize, loaded),
      status: "INCOMPLETE",
      locked,
      resume: (file) => {
        if (file.name !== name || file.size !== expectedSize) {
          this.on.onError?.(`Expected ${name} (${formatBytes(expectedSize)})`);
          return;
        }
        this.#transition(id, this.PENDING(id, file, loaded));
        this.#drain();
      },
    };
  }

  PENDING(id, file, loaded) {
    return {
      ...this.#base(id, file.name, file.size, loaded),
      status: "PENDING",
      file,
    };
  }

  UPLOADING(id, file, loaded, controller) {
    return {
      ...this.#base(id, file.name, file.size, loaded),
      status: "UPLOADING",
      file,
      abort: () => controller.abort(),
    };
  }

  DONE(id, file, loaded) {
    return {
      ...this.#base(id, file.name, file.size, loaded),
      status: "DONE",
      file,
    };
  }

  LOCKED(id, file, loaded) {
    return {
      ...this.#base(id, file.name, file.size, loaded),
      status: "LOCKED",
      file,
      retry: () => {
        this.#transition(id, this.PENDING(id, file, loaded));
        this.#drain();
      },
    };
  }

  ERROR(id, file, loaded, message) {
    return {
      ...this.#base(id, file.name, file.size, loaded),
      status: "ERROR",
      file,
      message,
      retry: () => {
        this.#transition(id, this.PENDING(id, file, loaded));
        this.#drain();
      },
    };
  }

  seed(entries, state = {}) {
    for (const e of entries || []) {
      const st = state[e.id] || {};
      const loaded = st.written || 0;
      if (loaded >= e.expectedSize) continue;

      const task = this.INCOMPLETE(
        e.id,
        e.name,
        e.expectedSize,
        loaded,
        !!st.locked,
      );
      this.tasks.set(e.id, task);
      this.on.onAdd?.(task);
    }
  }

  async submit(files) {
    const fresh = [];

    for (const file of files) {
      const stale = Array.from(this.tasks.values()).find(
        (t) =>
          t.status === "INCOMPLETE" &&
          t.name === file.name &&
          t.expectedSize === file.size,
      );
      if (stale) {
        stale.resume(file);
      } else {
        fresh.push(file);
      }
    }

    if (fresh.length) {
      const entries = await registerFiles(this.shareID, fresh);
      entries.forEach((e, i) => {
        const task = this.PENDING(e.id, fresh[i], 0);
        this.tasks.set(e.id, task);
        this.on.onAdd?.(task);
      });
    }
    this.#drain();
  }

  #transition(id, nextState) {
    const prev = this.tasks.get(id)?.status;
    this.tasks.set(id, nextState);
    this.on.onStatus?.(nextState, prev);
  }

  async #deleteFromServer(id) {
    const task = this.tasks.get(id);
    if (!task) return;

    if (task.status === "UPLOADING") task.abort();

    try {
      const res = await fetch(`/api/shares/${this.shareID}/files/${id}`, {
        method: "DELETE",
      });
      if (!res.ok) throw new Error(`delete failed: ${res.status}`);
      this.tasks.delete(id);
      this.on.onRemove?.(id);
    } catch (err) {
      this.on.onError?.(err.message);
    }
  }

  async #drain() {
    if (Array.from(this.tasks.values()).some((t) => t.status === "UPLOADING"))
      return;

    let completed = 0;
    let pendingTask;

    while (
      (pendingTask = Array.from(this.tasks.values()).find(
        (t) => t.status === "PENDING",
      ))
    ) {
      const controller = new AbortController();
      const uploadingTask = this.UPLOADING(
        pendingTask.id,
        pendingTask.file,
        pendingTask.loaded,
        controller,
      );
      this.#transition(pendingTask.id, uploadingTask);

      try {
        const outcome = await streamFile(
          this.shareID,
          uploadingTask.id,
          uploadingTask.file,
          (bytes) => {
            uploadingTask.loaded = bytes;
            this.on.onProgress?.(uploadingTask);
          },
          controller.signal,
        );
        if (outcome === "locked") {
          this.#transition(
            uploadingTask.id,
            this.LOCKED(
              uploadingTask.id,
              uploadingTask.file,
              uploadingTask.loaded,
            ),
          );
        } else {
          this.#transition(
            uploadingTask.id,
            this.DONE(
              uploadingTask.id,
              uploadingTask.file,
              uploadingTask.loaded,
            ),
          );
          completed++;
        }
      } catch (err) {
        if (!controller.signal.aborted) {
          this.#transition(
            uploadingTask.id,
            this.ERROR(
              uploadingTask.id,
              uploadingTask.file,
              uploadingTask.loaded,
              err.message,
            ),
          );
          this.on.onError?.(
            `failed to upload ${uploadingTask.name}: ${err.message}`,
          );
        }
      }
    }

    if (completed) this.on.onDrained?.();
  }
}
