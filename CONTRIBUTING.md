# Contributing to SysTrack

Thank you for your interest in contributing to SysTrack! This document provides guidelines and information for contributors.

## 🚀 Getting Started

### Prerequisites
- Go 1.19 or higher
- MySQL 8.0 or higher
- Git
- Basic understanding of Go, HTML, CSS, and JavaScript

### Development Setup

1. **Fork and Clone**
```bash
git clone https://github.com/yourusername/systrack.git
cd systrack
```

2. **Install Dependencies**
```bash
go mod tidy
```

3. **Database Setup**
```sql
CREATE DATABASE systrack;
```

4. **Environment Configuration**
```bash
cp env.example .env
# Edit .env with your database credentials
```

5. **Run the Application**
```bash
go run ./cmd/systrack
```

## 📋 Development Guidelines

### Code Style

#### Go Code
- Follow the [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- Use `gofmt` to format code
- Use meaningful variable and function names
- Add comments for exported functions and types
- Keep functions small and focused

#### Frontend Code
- Use Alpine.js for reactive components
- Follow Tailwind CSS utility-first approach
- Use semantic HTML elements
- Ensure accessibility compliance
- Add proper error handling

### Project Structure
```
systrack/
├── cmd/systrack/          # Main application
├── internal/
│   ├── handlers/          # HTTP handlers
│   ├── ping/             # Monitoring engine
│   ├── ws/               # WebSocket hub
│   └── auth/             # Authentication
├── static/               # Static assets
├── templates/           # HTML templates
└── docs/               # Documentation
```

### Database Changes
- Create migration scripts for schema changes
- Test migrations on sample data
- Update documentation for new fields
- Ensure backward compatibility

## 🐛 Bug Reports

### Before Submitting
1. Check existing issues
2. Test with the latest version
3. Reproduce the issue
4. Gather relevant information

### Bug Report Template
```markdown
**Describe the bug**
A clear description of what the bug is.

**To Reproduce**
Steps to reproduce the behavior:
1. Go to '...'
2. Click on '....'
3. Scroll down to '....'
4. See error

**Expected behavior**
What you expected to happen.

**Screenshots**
If applicable, add screenshots.

**Environment:**
- OS: [e.g. Windows 10, Ubuntu 20.04]
- Go version: [e.g. 1.19.0]
- MySQL version: [e.g. 8.0.25]
- Browser: [e.g. Chrome 91, Firefox 89]

**Additional context**
Any other context about the problem.
```

## ✨ Feature Requests

### Before Submitting
1. Check existing feature requests
2. Consider the project's scope
3. Think about implementation complexity
4. Consider user impact

### Feature Request Template
```markdown
**Is your feature request related to a problem?**
A clear description of what the problem is.

**Describe the solution you'd like**
A clear description of what you want to happen.

**Describe alternatives you've considered**
Alternative solutions or features you've considered.

**Additional context**
Any other context or screenshots about the feature request.
```

## 🔧 Pull Requests

### Before Submitting
1. Create a feature branch
2. Write tests for new functionality
3. Update documentation
4. Ensure all tests pass
5. Test manually

### Pull Request Template
```markdown
**Description**
Brief description of changes.

**Type of Change**
- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change
- [ ] Documentation update

**Testing**
- [ ] Unit tests pass
- [ ] Integration tests pass
- [ ] Manual testing completed

**Screenshots**
If applicable, add screenshots.

**Checklist**
- [ ] Code follows style guidelines
- [ ] Self-review completed
- [ ] Documentation updated
- [ ] No breaking changes
```

### Review Process
1. Automated checks must pass
2. Code review by maintainers
3. Manual testing
4. Documentation review
5. Final approval

## 🧪 Testing

### Unit Tests
```bash
go test ./...
```

### Integration Tests
```bash
go test -tags=integration ./...
```

### Coverage
```bash
go test -cover ./...
```

### Manual Testing
- Test all user flows
- Verify error handling
- Check responsive design
- Test with different browsers

## 📚 Documentation

### Code Documentation
- Add comments for exported functions
- Document complex algorithms
- Include usage examples
- Update README for new features

### API Documentation
- Document all endpoints
- Include request/response examples
- Add error codes and messages
- Update OpenAPI spec if applicable

## 🏷️ Release Process

### Versioning
We follow [Semantic Versioning](https://semver.org/):
- **MAJOR**: Breaking changes
- **MINOR**: New features (backward compatible)
- **PATCH**: Bug fixes (backward compatible)

### Release Checklist
- [ ] Update version numbers
- [ ] Update CHANGELOG.md
- [ ] Update documentation
- [ ] Run full test suite
- [ ] Create release notes
- [ ] Tag release
- [ ] Publish release

## 🤝 Community Guidelines

### Code of Conduct
- Be respectful and inclusive
- Welcome newcomers
- Focus on constructive feedback
- Help others learn and grow

### Communication
- Use clear and concise language
- Provide context for questions
- Be patient with responses
- Use appropriate channels

### Recognition
- Contributors will be credited
- Significant contributions may be highlighted
- Long-term contributors may be invited as maintainers

## 🆘 Getting Help

### Resources
- [Documentation](README.md)
- [GitHub Issues](https://github.com/yourusername/systrack/issues)
- [GitHub Discussions](https://github.com/yourusername/systrack/discussions)

### Contact
- Create an issue for bugs
- Use discussions for questions
- Tag maintainers for urgent issues

## 📝 License

By contributing to SysTrack, you agree that your contributions will be licensed under the MIT License.

## 🙏 Thank You

Thank you for contributing to SysTrack! Your contributions help make network monitoring more accessible and powerful for everyone.

---

**Happy Contributing! 🚀**
