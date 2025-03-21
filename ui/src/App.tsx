import { useState, KeyboardEvent, ChangeEvent, DragEvent, useRef } from "react";
import { Container, Row, Col, Form, Button, ListGroup, Card, Table, Tabs, Tab } from "react-bootstrap";
import "bootstrap/dist/css/bootstrap.min.css";
import "./App.css";

function App() {
  const [categories, setCategories] = useState<string[]>(["General", "Technology", "Science", "History"]);
  const [newCategory, setNewCategory] = useState<string>("");
  const [searchQuery, setSearchQuery] = useState<string>("");
  const [searchResults, setSearchResults] = useState<any[]>([]);
  const [selectedCategory, setSelectedCategory] = useState<string>("General");
  
  // File upload states
  const [uploadedFiles, setUploadedFiles] = useState<File[]>([]);
  const [dragActive, setDragActive] = useState<boolean>(false);
  const [directText, setDirectText] = useState<string>("");
  const [activeUploadTab, setActiveUploadTab] = useState<string>("drag-drop");
  const fileInputRef = useRef<HTMLInputElement>(null);

  const handleAddCategory = () => {
    if (newCategory.trim() !== "" && !categories.includes(newCategory.trim())) {
      setCategories([...categories, newCategory.trim()]);
      setNewCategory("");
    }
  };

  const handleSearch = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter" && searchQuery.trim() !== "") {
      e.preventDefault();
      // Mock search results - in a real app, this would call an API
      const mockResults = [
        { id: 1, title: `Result 1 for "${searchQuery}" in ${selectedCategory}`, description: "This is a description for the first search result." },
        { id: 2, title: `Result 2 for "${searchQuery}" in ${selectedCategory}`, description: "This is a description for the second search result." },
        { id: 3, title: `Result 3 for "${searchQuery}" in ${selectedCategory}`, description: "This is a description for the third search result." },
      ];
      setSearchResults(mockResults);
    }
  };
  
  // File upload handlers
  const handleDrag = (e: DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    e.stopPropagation();
    
    if (e.type === "dragenter" || e.type === "dragover") {
      setDragActive(true);
    } else if (e.type === "dragleave") {
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
  
  const handleProcessFiles = () => {
    // In a real app, this would process the files or direct text
    console.log("Processing files:", uploadedFiles);
    console.log("Processing direct text:", directText);
    
    // Clear the upload area after processing
    if (activeUploadTab === "direct-input" && directText.trim() !== "") {
      alert(`Text processed: ${directText.substring(0, 50)}${directText.length > 50 ? "..." : ""}`);
      setDirectText("");
    } else if (uploadedFiles.length > 0) {
      alert(`${uploadedFiles.length} file(s) processed`);
      setUploadedFiles([]);
    }
  };

  return (
    <Container fluid className="app-container bg-dark text-light min-vh-100 p-0">
      <Row className="m-0">
        {/* Left Navigation Panel */}
        <Col md={3} className="sidebar p-4">
          <Card bg="dark" text="light" className="mb-4 rounded-3 border-secondary">
            <Card.Header className="border-secondary">
              <h4>Categories</h4>
            </Card.Header>
            <Card.Body>
              <ListGroup variant="flush">
                {categories.map((category, index) => (
                  <ListGroup.Item 
                    key={index} 
                    action 
                    variant="dark"
                    active={selectedCategory === category}
                    onClick={() => setSelectedCategory(category)}
                    className="rounded-2 mb-1 border-secondary"
                  >
                    {category}
                  </ListGroup.Item>
                ))}
              </ListGroup>
            </Card.Body>
          </Card>
          
          <Card bg="dark" text="light" className="rounded-3 border-secondary">
            <Card.Header className="border-secondary">
              <h5>Add New Category</h5>
            </Card.Header>
            <Card.Body>
              <Form.Group className="mb-3">
                <Form.Control
                  type="text"
                  placeholder="New category name"
                  value={newCategory}
                  onChange={(e) => setNewCategory(e.target.value)}
                  className="bg-dark text-light rounded-3 border-secondary"
                />
              </Form.Group>
              <Button 
                variant="outline-light" 
                onClick={handleAddCategory}
                className="w-100 rounded-3"
              >
                Add Category
              </Button>
            </Card.Body>
          </Card>
        </Col>
        
        {/* Main Content Area */}
        <Col md={9} className="main-content p-4">
          <Card bg="dark" text="light" className="mb-4 rounded-3 border-secondary">
            <Card.Body>
              <h2 className="mb-4 text-center">Vector Search</h2>
              <Form>
                <Form.Group className="mb-3">
                  <Form.Control
                    type="text"
                    placeholder="Enter your search query..."
                    value={searchQuery}
                    onChange={(e) => setSearchQuery(e.target.value)}
                    onKeyDown={handleSearch}
                    className="form-control-lg bg-dark text-light rounded-3 border-secondary"
                  />
                  <div className="text-muted mt-2 small">
                    Searching in category: <span className="fw-bold">{selectedCategory}</span>
                  </div>
                </Form.Group>
              </Form>
            </Card.Body>
          </Card>
          
          {/* Search Results */}
          {searchResults.length > 0 && (
            <Card bg="dark" text="light" className="rounded-3 border-secondary mb-4">
              <Card.Header className="border-secondary">
                <h4>Search Results</h4>
              </Card.Header>
              <Card.Body>
                <Table variant="dark" className="border-secondary">
                  <tbody>
                    {searchResults.map((result) => (
                      <tr key={result.id}>
                        <td>
                          <div className="search-result p-2 rounded-3">
                            <h5 className="text-primary">{result.title}</h5>
                            <p className="mb-0">{result.description}</p>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </Table>
              </Card.Body>
            </Card>
          )}
          
          {/* File Upload Section */}
          <Card bg="dark" text="light" className="rounded-3 border-secondary">
            <Card.Header className="border-secondary">
              <h4>Upload Content</h4>
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
                disabled={(activeUploadTab === "drag-drop" && uploadedFiles.length === 0) || 
                         (activeUploadTab === "direct-input" && directText.trim() === "")}
              >
                Process Content
              </Button>
            </Card.Body>
          </Card>
        </Col>
      </Row>
    </Container>
  );
}

export default App;
