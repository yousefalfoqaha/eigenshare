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

async function readErrorMessage(res) {
  try {
    const payload = await res.json();
    if (payload?.message) return payload.message;
  } catch {}
  return `HTTP ${res.status}`;
}

async function registerFiles(shareID, files) {
  try {
    const res = await fetch(`/api/shares/${shareID}/files`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        files: files.map((f) => ({ name: f.name, expectedSize: f.size })),
      }),
    });
    if (!res.ok) throw new Error(await readErrorMessage(res));

    return { status: "success", entries: (await res.json()) || [] };
  } catch (ex) {
    return { status: "error", message: ex.message };
  }
}

async function deleteFile(shareID, fileID) {
  try {
    const res = await fetch(`/api/shares/${shareID}/files/${fileID}`, {
      method: "DELETE",
    });
    if (!res.ok) throw new Error(await readErrorMessage(res));

    return { status: "success" };
  } catch (ex) {
    return { status: "error", message: ex.message };
  }
}

async function getUploadState(shareID, fileID, signal) {
  try {
    const res = await fetch(`/api/shares/${shareID}/files/${fileID}/stream`, {
      method: "HEAD",
      signal,
    });
    if (!res.ok) throw new Error(await readErrorMessage(res));

    const header = res.headers.get("Upload-Offset");
    if (header === null) throw new Error("Missing Upload-Offset header");

    const offset = Number(header);
    if (res.headers.get("Upload-Locked") !== null) {
      return { status: "locked", offset };
    }

    return { status: "success", offset };
  } catch (ex) {
    if (ex.name === "AbortError") return { status: "aborted" };
    return { status: "error", message: ex.message };
  }
}

function uploadChunk(shareID, fileID, file, offset, onProgress, signal) {
  if (signal.aborted) return Promise.resolve({ status: "aborted" });
  if (offset >= file.size) return Promise.resolve({ status: "complete" });

  return new Promise((resolve) => {
    const chunkSize = 50 * 1024 * 1024;
    const data = file.slice(offset, offset + chunkSize);

    const xhr = new XMLHttpRequest();
    xhr.open("PUT", `/api/shares/${shareID}/files/${fileID}/stream`);
    xhr.setRequestHeader("Content-Type", "application/octet-stream");
    xhr.setRequestHeader("Upload-Offset", String(offset));

    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) onProgress(offset + e.loaded);
    };

    xhr.onload = () => {
      if (xhr.status === 423) {
        return resolve({ status: "locked", offset });
      }

      if (xhr.status === 204 || xhr.status === 409) {
        const header = xhr.getResponseHeader("Upload-Offset");
        if (header !== null) {
          return resolve({ status: "success", offset: Number(header) });
        }
        return resolve({
          status: "error",
          code: xhr.status,
          message: `Server returned ${xhr.status} but missing Upload-Offset header`,
        });
      }

      let errorMsg = "";
      try {
        const payload = JSON.parse(xhr.responseText);
        if (payload.message) errorMsg = payload.message;
      } catch {}

      resolve({
        status: "error",
        code: xhr.status,
        message: errorMsg
          ? `upload rejected: ${errorMsg}`
          : `upload rejected (HTTP ${xhr.status})`,
      });
    };

    xhr.onerror = () =>
      resolve({ status: "error", code: 0, message: "network error" });
    xhr.onabort = () => resolve({ status: "aborted" });

    signal.addEventListener("abort", () => xhr.abort(), { once: true });
    xhr.send(data);
  });
}

async function uploadFile(shareID, fileID, file, onProgress, signal) {
  const state = await getUploadState(shareID, fileID, signal);
  if (state.status !== "success") return state;

  let offset = state.offset;
  let attempts = 0;

  onProgress(offset);

  while (true) {
    const result = await uploadChunk(
      shareID,
      fileID,
      file,
      offset,
      onProgress,
      signal,
    );

    if (result.status === "complete" || result.status === "aborted") {
      return result;
    }

    if (result.status === "success") {
      offset = result.offset;
      attempts = 0;

      onProgress(offset);
      continue;
    }

    if (result.status === "error") {
      if (
        result.code >= 400 &&
        result.code < 500 &&
        result.code !== 423 &&
        result.code !== 409
      ) {
        return result;
      }
    }

    if (++attempts > 3) return result;
    await new Promise((r) => setTimeout(r, 1000));
  }
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
      const result = await registerFiles(this.shareID, fresh);
      if (result.status === "error") {
        this.on.onError?.(`failed to register files: ${result.message}`);
        return;
      }

      result.entries.forEach((e, i) => {
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

    const result = await deleteFile(this.shareID, id);
    if (result.status === "error") {
      this.on.onError?.(`failed to delete ${task.name}: ${result.message}`);
      return;
    }

    this.tasks.delete(id);
    this.on.onRemove?.(id);
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
        const outcome = await uploadFile(
          this.shareID,
          uploadingTask.id,
          uploadingTask.file,
          (bytes) => {
            uploadingTask.loaded = bytes;
            this.on.onProgress?.(uploadingTask);
          },
          controller.signal,
        );

        switch (outcome.status) {
          case "complete":
            this.#transition(
              uploadingTask.id,
              this.DONE(
                uploadingTask.id,
                uploadingTask.file,
                uploadingTask.loaded,
              ),
            );
            completed++;
            break;

          case "locked":
            this.#transition(
              uploadingTask.id,
              this.LOCKED(
                uploadingTask.id,
                uploadingTask.file,
                uploadingTask.loaded,
              ),
            );
            break;

          case "aborted":
            break;

          case "error":
            this.#transition(
              uploadingTask.id,
              this.ERROR(
                uploadingTask.id,
                uploadingTask.file,
                uploadingTask.loaded,
                outcome.message,
              ),
            );
            this.on.onError?.(
              `failed to upload ${uploadingTask.name}: ${outcome.message}`,
            );
            break;
        }
      } catch (ex) {
        if (!controller.signal.aborted) {
          this.#transition(
            uploadingTask.id,
            this.ERROR(
              uploadingTask.id,
              uploadingTask.file,
              uploadingTask.loaded,
              "Unexpected internal error",
            ),
          );
          this.on.onError?.(
            `Unexpected error uploading ${uploadingTask.name}: ${ex.message}`,
          );
        }
      }
    }

    if (completed) this.on.onDrained?.();
  }
}
