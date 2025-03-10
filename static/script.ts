document.addEventListener("DOMContentLoaded", () => {
    const textInput = document.getElementById("textInput") as HTMLInputElement;
    const outputText = document.getElementById("outputText") as HTMLParagraphElement;

    textInput.addEventListener("input", () => {
        outputText.textContent = `You typed: ${textInput.value}`;
    });
});
