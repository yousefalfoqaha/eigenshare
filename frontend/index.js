const CHUNK_SIZE_MB = 5;
const BASE_URL = "http://localhost:8080";

function main() {
  const form = document.getElementById("file-form");

  if (!form) return;

  form.addEventListener("submit", async (e) => {
    e.preventDefault();

    const formData = new FormData(form);
    const file = formData.get("filename");

    const body = {
      handle: "hello",
      files: [{ name: file.name, size: file.size }],
    };

    let share;
    try {
      const response = await fetch(BASE_URL + "/shares", {
        method: "POST",
        body: JSON.stringify(body),
      });

      if (!response.ok) {
        throw new Error(`Response status: ${response.status}`);
      }

      share = await response.json();
    } catch (error) {
      console.error(error.message);
    }

    uploadFileSequentially(share, file, 0);
  });
}

async function uploadFileSequentially(share, file, index) {
  const chunkSize = 1024 * 1024 * CHUNK_SIZE_MB;
  let offset = 0;
  let chunkIndex = 0;

  while (offset < file.size) {
    const chunk = file.slice(offset, offset + chunkSize);

    try {
      const response = await fetch(
        BASE_URL + "/shares/upload?shareId=" + share.id,
        {
          method: "POST",
          body: chunk,
          headers: {
            "Content-Type": "application/octet-stream",
            "X-Chunk-Index": chunkIndex.toString(),
            "X-File-Name": file.name,
            "x-File-Index": index.toString(),
          },
        },
      );

      if (!response.ok) {
        throw new Error(`Response status: ${response.status}`);
      }

      const result = await response.json();

      console.log(result);
    } catch (error) {
      console.error(error.message);
    }

    offset += chunkSize;
    chunkIndex++;
  }
}

main();
