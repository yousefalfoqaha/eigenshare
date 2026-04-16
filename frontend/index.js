function main() {
  const form = document.getElementById("file-form");

  if (!form) return;

  form.addEventListener("submit", (e) => {
    e.preventDefault();

    const formData = new FormData(form);
    const file = formData.get("filename");
    console.log("File to upload:", file);
  });
}

main();
