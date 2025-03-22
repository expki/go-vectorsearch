import mammoth from "mammoth";

export default async function extractTextFromDOCX(file: File): Promise<string> {
    const reader = new FileReader();
    reader.readAsArrayBuffer(file);
    return new Promise((resolve) => {
        reader.onload = async () => {
            if (reader.result === null) return;
            if (typeof(reader.result) === 'string') {
                resolve(reader.result);
                return;
            };
            const { value } = await mammoth.extractRawText({ arrayBuffer: reader.result });
            resolve(value);
        };
    });
}