import { useState, ChangeEvent, DragEvent, useRef } from 'react';
import { Form, Button, ListGroup, Card, Tabs, Tab, Spinner } from 'react-bootstrap';

import { Upload, DocumentUpload } from './api/upload';
import decodeDocx from './tools/doc';
import decodePdf from './tools/pdf';

type Props = {
  owner: string,
  category: string,
}

function Content({ owner, category }: Props) {
  // File upload states
  const [uploadedFiles, setUploadedFiles] = useState<File[]>([]);
  const [dragActive, setDragActive] = useState<boolean>(false);
  const [directText, setDirectText] = useState<string>('');
  const [activeUploadTab, setActiveUploadTab] = useState<string>('direct-input');
  const [uploading, setUploading] = useState<boolean>(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  
  // File upload handlers
  const handleDrag = (e: DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    e.stopPropagation();
    
    if (e.type === 'dragenter' || e.type === 'dragover') {
      setDragActive(true);
    } else if (e.type === 'dragleave') {
      setDragActive(false);
    }
  };
  
  const handleDrop = (e: DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    e.stopPropagation();
    setDragActive(false);
    
    if (e.dataTransfer.files && e.dataTransfer.files.length > 0) {
      const newFiles = Array.from(e.dataTransfer.files);
      setUploadedFiles(prevFiles => [...prevFiles, ...newFiles]);
    }
  };
  
  const handleFileChange = (e: ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files.length > 0) {
      const newFiles = Array.from(e.target.files);
      setUploadedFiles(prevFiles => [...prevFiles, ...newFiles]);
    }
  };
  
  const handleButtonClick = () => {
    if (fileInputRef.current) {
      fileInputRef.current.click();
    }
  };
  
  const handleRemoveFile = (index: number) => {
    setUploadedFiles(prevFiles => prevFiles.filter((_, i) => i !== index));
  };
  
  const handleProcessFiles = async () => {
    setUploading(true);
    // In a real app, this would process the files or direct text
    console.log("Processing files:", uploadedFiles);
    console.log("Processing direct text:", directText);
    
    // Clear the upload area after processing
    if (activeUploadTab === "direct-input" && directText.trim() !== "") {
      Upload({
        owner: owner,
        category: category,
        prefix: category,
        documents: [{document: directText}],
      }).then(() => console.log(`Text processed: ${directText.substring(0, 50)}${directText.length > 50 ? "..." : ""}`));
      setDirectText('');
    } else if (uploadedFiles.length > 0) {
      let documents: Array<DocumentUpload> = [];
      for (let file of uploadedFiles) {
        if (file.name.endsWith('.pdf')) {
          documents.push({document: (await decodePdf(file)).substring(0, 5000)});
        } else if (file.name.endsWith('.docx')) {
          documents.push({document: (await decodeDocx(file)).substring(0, 5000)});
        } else {
          documents.push({document: (await file.text()).substring(0, 5000)});
        }
      }
      Upload({
        owner: owner,
        category: category,
        prefix: category,
        documents: documents,
      }).then(() => console.log(`${uploadedFiles.length} file(s) processed`));
      setUploadedFiles([]);
    }
    
    setUploading(false);
  };

  return (
    <Card bg="dark" text="light" className="rounded-3 border-secondary">
      <Card.Header className="border-secondary">
        <h4>Upload Content</h4>
        <div className="text-muted mt-2 small">
          Content is secure to your browser
        </div>
      </Card.Header>
      <Card.Body>
        <Tabs
          activeKey={activeUploadTab}
          onSelect={(k) => k && setActiveUploadTab(k)}
          className="mb-3 upload-tabs"
          variant="pills"
        >
          {/* Drag and Drop Tab */}
          <Tab eventKey="drag-drop" title="Drag & Drop Files">
            <div 
              className={`file-upload-area p-4 rounded-3 mb-3 ${dragActive ? "drag-active" : ""}`}
              onDragEnter={handleDrag}
              onDragOver={handleDrag}
              onDragLeave={handleDrag}
              onDrop={handleDrop}
            >
              <div className="text-center">
                <h5>Drag & Drop Files Here</h5>
                <p className="text-muted">or</p>
                <Button 
                  variant="outline-light" 
                  className="rounded-3 mt-2"
                  onClick={handleButtonClick}
                >
                  Browse Files
                </Button>
                <Form.Control
                  type="file"
                  ref={fileInputRef}
                  accept=".txt,.pdf,.doc"
                  onChange={handleFileChange}
                  multiple
                  className="d-none"
                />
              </div>
            </div>
            
            {/* File List */}
            {uploadedFiles.length > 0 && (
              <div className="uploaded-files mb-3">
                <h6 className="mb-2">Uploaded Files:</h6>
                <ListGroup variant="flush">
                  {uploadedFiles.map((file, index) => (
                    <ListGroup.Item 
                      key={index} 
                      variant="dark"
                      className="d-flex justify-content-between align-items-center rounded-2 mb-1 border-secondary"
                    >
                      <div>
                        {file.name} <span className="text-muted">({(file.size / 1024).toFixed(1)} KB)</span>
                      </div>
                      <Button 
                        variant="outline-danger" 
                        size="sm" 
                        className="rounded-circle"
                        onClick={() => handleRemoveFile(index)}
                      >
                        X
                      </Button>
                    </ListGroup.Item>
                  ))}
                </ListGroup>
              </div>
            )}
          </Tab>
          
          {/* Direct Text Input Tab */}
          <Tab eventKey="direct-input" title="Direct Text Input">
            <Form.Group className="mb-3">
              <Form.Control
                as="textarea"
                rows={6}
                placeholder="Enter or paste your text here..."
                value={directText}
                onChange={(e) => setDirectText(e.target.value)}
                className="bg-dark text-light rounded-3 border-secondary"
              />
            </Form.Group>
          </Tab>
        </Tabs>
        
        {/* Process Button */}
        <Button 
          variant="primary" 
          className="w-100 rounded-3 mt-2"
          onClick={handleProcessFiles}
          disabled={
            uploading ||
            (activeUploadTab === "drag-drop" && uploadedFiles.length === 0) || 
            (activeUploadTab === "direct-input" && directText.trim() === "")
          }
        >
          {uploading ?
            <Spinner animation="border" role="status">
              <span className="visually-hidden">Uploading...</span>
            </Spinner>
            : 
            <>Upload</>
          }
        </Button>
      </Card.Body>
    </Card>
  );
}

export default Content;
