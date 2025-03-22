import { useState } from 'react';
import { Form, Card, Table, Button, Row, Col, Spinner } from 'react-bootstrap';

import { Search } from './api/search';
import { DeleteDocument } from './api/delete';

type Props = {
  owner: string,
  category: string,
}

function App({ owner, category }: Props) {
  const [searchQuery, setSearchQuery] = useState<string>('');
  const [searchResults, setSearchResults] = useState<Array<result>>([]);
  const [searching, setSearching] = useState<boolean>(false);

  const handleSearch = () => {
    setSearching(true);
    Search({
      owner: owner,
      category: category,
      prefix: category,
      text: searchQuery.trim(),
      count: 3,
    }).then((res) => {
      const documents = res?.documents ?? [];
      const results: Array<result> = documents.map((document, idx) => ({
        id: document.document_id,
        title: `Result ${idx+1} has ${(100 * document.document_similarity).toFixed(2)}% similarity`,
        description: String(document.document),
      }));
      setSearchResults(results);
      setSearching(false);
    });
  };

  const handleDeleteDocument = (documentID: number) => {
    DeleteDocument(owner, category, documentID);
    const updatedSearchResults = [...searchResults].filter((item) => item.id !== documentID);
    setSearchResults(updatedSearchResults);
  }

  return (
    <>
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
                onKeyDown={(e) => {
                  if (e.key !== 'Enter') {
                    return;
                  }
                  e.preventDefault();
                  handleSearch();
                }}
                className="form-control-lg bg-dark text-light rounded-3 border-secondary"
              />
              <div className="text-muted mt-2 small">
                Searching in category: <span className="fw-bold">{category}</span>
              </div>
              <Button 
                variant="primary" 
                className="w-100 rounded-3 mt-2"
                onClick={handleSearch}
                disabled={searching}
              >
                {searching ?
                  <Spinner animation="border" role="status">
                    <span className="visually-hidden">Searching...</span>
                  </Spinner>
                  : 
                  <>Search</>
                }
              </Button>
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
                        <Row>
                          <Col>
                            <h5 className="text-primary">{result.title}</h5>
                          </Col>
                          <Col xs="auto">
                            <Button 
                              variant="outline-danger" 
                              size="sm" 
                              className="rounded-circle"
                              onClick={() => handleDeleteDocument(result.id)}
                            >
                              X
                            </Button>
                          </Col>
                        </Row>
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
    </>
  );
}

export default App;

type result = {
  id: number,
  title: string,
  description: string,
}
