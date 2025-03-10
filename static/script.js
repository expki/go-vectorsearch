document.addEventListener("DOMContentLoaded", () => {
    const textInput = document.getElementById("textInput");
    const outputText = document.getElementById("outputText");

    textInput.addEventListener("input", () => {
        outputText.textContent = `You typed: ${textInput.value}`;
    });
});
