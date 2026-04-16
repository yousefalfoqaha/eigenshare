function main() {
  const form = document.getElementById("file-form");

  if (!form) return;

  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    const formData = new FormData(form);
    const file = formData.get("filename");

    const url = "http://localhost:8080/shares";
    const body = {
      handle: "hello",
      files: [{ name: file.name, size: file.size }],
    };

    console.log(body);
    try {
      const response = await fetch(url, {
        method: "POST",
        body: JSON.stringify(body),
      });

      if (!response.ok) {
        throw new Error(`Response status: ${response.status}`);
      }

      const result = await response.json();

      console.log(result);
    } catch (error) {
      console.error(error.message);
    }
  });
}

main();
