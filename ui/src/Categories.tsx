import { useState } from "react";
import { Form, Button, ListGroup, Card, Row, Col } from "react-bootstrap";

import { DeleteCategory } from "./api/delete";

type Props = {
  owner: string,
  selectedCategory: string,
  categories: Array<string>,
  setSelectedCategory(category: string): void;
  setCategories(categories: Array<string>): void,
}

function Categories({
  owner, selectedCategory, categories, setSelectedCategory, setCategories,
}: Props) {
  const [newCategory, setNewCategory] = useState<string>('');

  const handleAddCategory = () => {
    const value = newCategory.trim();
    if (value === '') {
      return;
    }
    if (categories.includes(value)) {
      return;
    }
    setCategories([...categories, value]);
    setNewCategory('');
  };

  const handleDeleteCategory = (category: string) => {
    DeleteCategory(owner, category);
    const updatedCategories = [...categories].filter((item) => item !== category);
    setCategories(updatedCategories);
    if (category === selectedCategory) setSelectedCategory(updatedCategories[0]);
  }

  return (
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
              <Row>
                <Col>
                  {category}
                </Col>
                <Col xs="auto">
                  <Button 
                    variant="outline-danger" 
                    size="sm" 
                    className="rounded-circle"
                    onClick={() => handleDeleteCategory(category)}
                    hidden={categories.length <= 1}
                  >
                    X
                  </Button>
                </Col>
              </Row>
            </ListGroup.Item>
          ))}
        </ListGroup>
      </Card.Body>
      <Card.Footer>
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
      </Card.Footer>
    </Card>
  );
}

export default Categories;
