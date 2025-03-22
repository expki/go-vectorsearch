import * as pdfjsLib from "pdfjs-dist";
import "pdfjs-dist/build/pdf.worker.mjs";

export default async function extractTextFromPDF(file: File): Promise<string> {
    const reader = new FileReader();
    reader.readAsArrayBuffer(file);
    return new Promise((resolve) => {
        reader.onload = async () => {
            if (reader.result === null) return;
            const pdf = await pdfjsLib.getDocument({ data: reader.result }).promise;
            let text = '';
            for (let i = 1; i <= pdf.numPages; i++) {
                const page = await pdf.getPage(i);
                const textContent = await page.getTextContent();
                text += textContent.items.map((item: any) => item?.str ?? '').join(' ') + '\n';
            }
            resolve(text);
        };
    });
}